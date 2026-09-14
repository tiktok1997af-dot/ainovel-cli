[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RepoRoot,
    [Parameter(Mandatory = $true)][string]$PackageArchivePath,
    [Parameter(Mandatory = $true)][string]$ChecksumManifestPath,
    [Parameter(Mandatory = $true)][string]$ReadinessExe,
    [Parameter(Mandatory = $true)][string]$ExpectedGitSha,
    [Parameter(Mandatory = $true)][string]$EvidenceDir,
    [Parameter(Mandatory = $true)][string]$RuntimeDir,
    [int]$ReadinessTimeoutSeconds = 60,
    [int]$RunTimeoutSeconds = 1200
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "D08 dual-provider packaged smoke FAIL: $Message"
}

function Resolve-GitBash {
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($null -eq $git -or [string]::IsNullOrWhiteSpace([string]$git.Source)) {
        Fail 'required command unavailable: git'
    }
    $gitCmdDir = Split-Path -Parent $git.Source
    $gitRoot = Split-Path -Parent $gitCmdDir
    foreach ($candidate in @(
        (Join-Path $gitRoot 'bin\bash.exe'),
        (Join-Path $gitRoot 'usr\bin\bash.exe')
    )) {
        if (Test-Path -LiteralPath $candidate) { return (Resolve-Path $candidate).Path }
    }
    Fail "Git for Windows bash.exe was not found under $gitRoot"
}

function Get-ProfileChrome([string]$ProfileDir) {
    if ([string]::IsNullOrWhiteSpace($ProfileDir)) { return @() }
    $needle = "--user-data-dir=$ProfileDir"
    return @(Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -and $_.CommandLine.Contains($needle) })
}

function Stop-ProfileChrome([string]$ProfileDir) {
    foreach ($proc in @(Get-ProfileChrome $ProfileDir)) {
        try { Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop } catch { }
    }
}

function Assert-NoProfileChrome([string]$ProfileDir, [string]$Label) {
    $left = @(Get-ProfileChrome $ProfileDir)
    if ($left.Count -ne 0) {
        Fail ("$Label browser profile still owns $($left.Count) Chrome process(es) after normal packaged shutdown")
    }
}

function Stop-PackagedProduction([string]$ExecutablePath) {
    if ([string]::IsNullOrWhiteSpace($ExecutablePath)) { return }
    $target = [IO.Path]::GetFullPath($ExecutablePath)
    Get-CimInstance Win32_Process -Filter "Name='ainovel-cli.exe'" -ErrorAction SilentlyContinue |
        Where-Object {
            try {
                $_.ExecutablePath -and ([IO.Path]::GetFullPath([string]$_.ExecutablePath) -eq $target)
            } catch {
                $false
            }
        } |
        ForEach-Object {
            try { Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop } catch { }
        }
}

function Get-SanitizedDiagnosticTail([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return '<stderr-missing>' }

    $diagnosticPattern = '(?i)(error|fail|fatal|panic|auth|required|config|browser|chrome|web|headless|host|engine|invalid|missing|not found|timeout|exit|cannot|unable|ready|denied|refused|unexpected)'
    $lines = @(Get-Content -LiteralPath $Path -Encoding UTF8 -Tail 80 -ErrorAction SilentlyContinue)
    $safe = New-Object System.Collections.Generic.List[string]
    $userHome = [Environment]::GetFolderPath('UserProfile')

    foreach ($raw in $lines) {
        $line = [string]$raw
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        if ($line -notmatch $diagnosticPattern) { continue }

        if (-not [string]::IsNullOrWhiteSpace($userHome)) {
            $line = $line.Replace($userHome, '<USERPROFILE>')
        }
        $line = [regex]::Replace($line, '(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b', '<redacted-email>')
        $line = [regex]::Replace($line, '(?i)\b(cookie|authorization|bearer|token|password|secret|api[_-]?key)\b\s*[:=]\s*[^\s,;]+', '$1=<redacted>')
        $line = [regex]::Replace($line, '(?i)(https?://[^\s?#]+)(?:\?[^\s#]*)?(?:#[^\s]*)?', '$1')
        if ($line.Length -gt 600) { $line = $line.Substring(0, 600) + '<truncated>' }
        $safe.Add($line)
    }

    if ($safe.Count -eq 0) { return '<no-sanitized-diagnostic-lines>' }
    return (($safe | Select-Object -Last 12) -join ' | ')
}

function Verify-ArchiveChecksum([string]$ArchivePath, [string]$ManifestPath) {
    $archiveName = [IO.Path]::GetFileName($ArchivePath)
    $matches = New-Object System.Collections.Generic.List[string]
    foreach ($raw in Get-Content -LiteralPath $ManifestPath -Encoding UTF8) {
        $fields = @(([string]$raw).Trim() -split '\s+' | Where-Object { $_ })
        if ($fields.Count -ne 2) { continue }
        if (([string]$fields[1]).TrimStart('*') -eq $archiveName) {
            $matches.Add(([string]$fields[0]).ToLowerInvariant())
        }
    }
    if ($matches.Count -ne 1 -or $matches[0] -notmatch '^[0-9a-f]{64}$') {
        Fail "checksum manifest does not contain one exact SHA-256 for $archiveName"
    }
    $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $matches[0]) { Fail "release archive SHA-256 mismatch" }
    return $actual
}

function Read-SessionProviderEvidence([string]$SessionDir) {
    $result = [ordered]@{
        architect_chatgpt = $false
        writer_gemini = $false
        architect_files = 0
        writer_files = 0
    }
    if (-not (Test-Path -LiteralPath $SessionDir)) { return $result }

    $files = @(Get-ChildItem -LiteralPath $SessionDir -Filter '*.jsonl' -File -ErrorAction SilentlyContinue)
    foreach ($file in $files) {
        $isArchitect = $file.Name.StartsWith('architect_', [StringComparison]::OrdinalIgnoreCase)
        $isWriter = $file.Name.StartsWith('writer-', [StringComparison]::OrdinalIgnoreCase)
        if ($isArchitect) { $result.architect_files++ }
        if ($isWriter) { $result.writer_files++ }
        if (-not $isArchitect -and -not $isWriter) { continue }

        foreach ($line in Get-Content -LiteralPath $file.FullName -Encoding UTF8) {
            if ([string]::IsNullOrWhiteSpace([string]$line)) { continue }
            try { $entry = [string]$line | ConvertFrom-Json } catch { continue }

            $metaProperty = $entry.PSObject.Properties['_meta']
            if ($null -eq $metaProperty -or $null -eq $metaProperty.Value) { continue }
            $meta = $metaProperty.Value
            $providerProperty = $meta.PSObject.Properties['provider']
            $modelProperty = $meta.PSObject.Properties['model']
            $provider = if ($null -eq $providerProperty) { '' } else { ([string]$providerProperty.Value).Trim().ToLowerInvariant() }
            $model = if ($null -eq $modelProperty) { '' } else { ([string]$modelProperty.Value).Trim().ToLowerInvariant() }

            if ($isArchitect -and $provider -eq 'chatgpt-web') {
                $result.architect_chatgpt = $true
            }
            if ($isWriter -and $provider -eq 'web' -and $model -eq 'gemini-web') {
                $result.writer_gemini = $true
            }
        }
    }
    return $result
}

if ($env:OS -ne 'Windows_NT') { Fail 'real interactive Windows is required' }
$gitBash = Resolve-GitBash

$RepoRoot = (Resolve-Path $RepoRoot).Path
$PackageArchivePath = (Resolve-Path $PackageArchivePath).Path
$ChecksumManifestPath = (Resolve-Path $ChecksumManifestPath).Path
$ReadinessExe = (Resolve-Path $ReadinessExe).Path
$EvidenceDir = [IO.Path]::GetFullPath($EvidenceDir)
$RuntimeDir = [IO.Path]::GetFullPath($RuntimeDir)
$ExpectedGitSha = $ExpectedGitSha.Trim().ToLowerInvariant()
if ($ExpectedGitSha -notmatch '^[0-9a-f]{40}$') { Fail 'expected SHA must be a full lowercase Git SHA' }

New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null
if (Test-Path -LiteralPath $RuntimeDir) { Remove-Item -LiteralPath $RuntimeDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $RuntimeDir | Out-Null

Push-Location $RepoRoot
try {
    $head = (git rev-parse HEAD).Trim().ToLowerInvariant()
    if ($LASTEXITCODE -ne 0 -or $head -ne $ExpectedGitSha) { Fail "candidate drift: $head" }
    & $gitBash scripts/w55-no-api-audit.sh
    if ($LASTEXITCODE -ne 0) { Fail 'exact-head W5.5 NO-API audit failed' }
} finally { Pop-Location }

$archiveSha = Verify-ArchiveChecksum $PackageArchivePath $ChecksumManifestPath
Add-Type -AssemblyName System.IO.Compression.FileSystem
$packageDir = Join-Path $RuntimeDir 'package'
New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
[System.IO.Compression.ZipFile]::ExtractToDirectory($PackageArchivePath, $packageDir)
$productionExe = Join-Path $packageDir 'ainovel-cli.exe'
if (-not (Test-Path -LiteralPath $productionExe)) { Fail 'packaged ainovel-cli.exe missing' }

$version = @(& $productionExe --version 2>&1 | ForEach-Object { ([string]$_).Trim() })
if ($LASTEXITCODE -ne 0 -or $version -notcontains ("commit: " + $ExpectedGitSha)) {
    Fail 'packaged executable provenance does not match exact candidate'
}

$geminiEvidence = Join-Path $EvidenceDir 'd08-gemini-readiness.json'
$chatgptEvidence = Join-Path $EvidenceDir 'd08-chatgpt-readiness.json'
Push-Location $RuntimeDir
try {
    & $ReadinessExe -timeout ("{0}s" -f $ReadinessTimeoutSeconds) -evidence $geminiEvidence
    if ($LASTEXITCODE -ne 0) { Fail 'Gemini READY + restart READY failed' }
    & $ReadinessExe -timeout ("{0}s" -f $ReadinessTimeoutSeconds) -site 'chatgpt-web' -profile-name 'ainovel-chatgpt-web' -evidence $chatgptEvidence
    if ($LASTEXITCODE -ne 0) { Fail 'ChatGPT READY + restart READY failed' }
} finally { Pop-Location }

$geminiReady = Get-Content -LiteralPath $geminiEvidence -Raw -Encoding UTF8 | ConvertFrom-Json
$chatgptReady = Get-Content -LiteralPath $chatgptEvidence -Raw -Encoding UTF8 | ConvertFrom-Json
foreach ($pair in @(@($geminiReady, 'gemini-web'), @($chatgptReady, 'chatgpt-web'))) {
    $ev = $pair[0]
    $site = [string]$pair[1]
    if ([string]$ev.schema -ne 'ainovel-w5e-readiness/1' -or [string]$ev.site -ne $site -or -not [bool]$ev.first_ready -or -not [bool]$ev.restart_ready) {
        Fail "$site sanitized readiness evidence is incomplete"
    }
}

$geminiProfile = [string]$geminiReady.profile_name
if ([string]::IsNullOrWhiteSpace($geminiProfile)) { $geminiProfile = 'default' }
$userHome = [Environment]::GetFolderPath('UserProfile')
$geminiProfileDir = Join-Path $userHome ('.ainovel\browser\profiles\' + $geminiProfile)
$chatgptProfileDir = Join-Path $userHome '.ainovel\browser\profiles\ainovel-chatgpt-web'
Assert-NoProfileChrome $geminiProfileDir 'Gemini readiness verifier'
Assert-NoProfileChrome $chatgptProfileDir 'ChatGPT readiness verifier'

$promptPath = Join-Path $RuntimeDir 'd08-prompt.txt'
@"
Hãy tạo một truyện ngắn thử nghiệm bằng tiếng Việt, chính xác 1 chương và kết thúc hoàn toàn trong chương đó. Hãy thực thi quy trình sáng tác bình thường của hệ thống: lập nền tảng, lập kế hoạch rồi viết và commit chương. Nội dung đơn giản: một người trực đêm ở ga tàu nhận được chiếc vé ghi chính tên mình. Không giải thích quy trình cho người dùng, không mở chương 2.
"@ | Set-Content -LiteralPath $promptPath -Encoding UTF8

$stdoutPath = Join-Path $EvidenceDir 'd08-packaged.stdout.log'
$stderrPath = Join-Path $EvidenceDir 'd08-packaged.stderr.log'
$runExitCodePath = Join-Path $RuntimeDir 'd08-packaged.exitcode.txt'
$runWrapperPath = Join-Path $RuntimeDir 'd08-packaged-wrapper.cmd'
$cmdLines = @(
    '@echo off',
    '"%D08_PRODUCTION_EXE%" --headless --prompt-file "%D08_PROMPT_PATH%"',
    'set "D08_EXIT_CODE=%ERRORLEVEL%"',
    '> "%D08_EXIT_CODE_PATH%" echo %D08_EXIT_CODE%',
    'exit /b %D08_EXIT_CODE%'
)
[System.IO.File]::WriteAllLines($runWrapperPath, $cmdLines, [System.Text.Encoding]::ASCII)
Remove-Item -LiteralPath $runExitCodePath -Force -ErrorAction SilentlyContinue

$cmdExe = [string]$env:ComSpec
if ([string]::IsNullOrWhiteSpace($cmdExe) -or -not (Test-Path -LiteralPath $cmdExe)) {
    Fail 'cmd.exe is required for reliable packaged exit-code capture'
}
$env:D08_PRODUCTION_EXE = $productionExe
$env:D08_PROMPT_PATH = $promptPath
$env:D08_EXIT_CODE_PATH = $runExitCodePath
$process = Start-Process -FilePath $cmdExe -ArgumentList @('/d', '/s', '/c', ('"' + $runWrapperPath + '"')) -WorkingDirectory $RuntimeDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -PassThru
Remove-Item Env:D08_PRODUCTION_EXE -ErrorAction SilentlyContinue
Remove-Item Env:D08_PROMPT_PATH -ErrorAction SilentlyContinue
Remove-Item Env:D08_EXIT_CODE_PATH -ErrorAction SilentlyContinue

try {
    if (-not $process.WaitForExit($RunTimeoutSeconds * 1000)) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        Stop-PackagedProduction $productionExe
        Fail 'packaged strict-role run timed out'
    }
    try { $process.WaitForExit() } catch { }

    if (-not (Test-Path -LiteralPath $runExitCodePath)) {
        Stop-PackagedProduction $productionExe
        Fail ("packaged strict-role exit-code sidecar missing; stderr=" + (Get-SanitizedDiagnosticTail $stderrPath))
    }
    $runExitRaw = (Get-Content -LiteralPath $runExitCodePath -Raw -Encoding UTF8).Trim()
    $runExitCode = 0
    if (-not [int]::TryParse($runExitRaw, [ref]$runExitCode)) {
        Stop-PackagedProduction $productionExe
        Fail ("packaged strict-role exit-code sidecar is invalid: " + $runExitRaw)
    }
    if ($runExitCode -ne 0) {
        Fail ("packaged strict-role run exited with code " + $runExitCode + "; stderr=" + (Get-SanitizedDiagnosticTail $stderrPath))
    }

    Start-Sleep -Seconds 2
    Assert-NoProfileChrome $geminiProfileDir 'Gemini packaged runtime'
    Assert-NoProfileChrome $chatgptProfileDir 'ChatGPT packaged runtime'
} finally {
    if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue }
}

$outputRoot = Join-Path $RuntimeDir 'output\novel'
$progressPath = Join-Path $outputRoot 'meta\progress.json'
if (-not (Test-Path -LiteralPath $progressPath)) { Fail 'progress.json missing after packaged run' }
$progress = Get-Content -LiteralPath $progressPath -Raw -Encoding UTF8 | ConvertFrom-Json
$completed = @($progress.completed_chapters | ForEach-Object { [int]$_ })
$chapters = @(Get-ChildItem -LiteralPath (Join-Path $outputRoot 'chapters') -Filter '*.md' -File -ErrorAction SilentlyContinue)
$providerEvidence = Read-SessionProviderEvidence (Join-Path $outputRoot 'meta\sessions\agents')

$boundaryDiagnostic = [ordered]@{
    schema = 'ainovel-d08-boundary-diagnostic/1'
    git_sha = $ExpectedGitSha
    phase = [string]$progress.phase
    current_chapter = [int]$progress.current_chapter
    in_progress_chapter = [int]$progress.in_progress_chapter
    total_chapters = [int]$progress.total_chapters
    completed_chapters = $completed
    chapter_file_count = [int]$chapters.Count
    architect_session_files = [int]$providerEvidence.architect_files
    writer_session_files = [int]$providerEvidence.writer_files
    architect_chatgpt = [bool]$providerEvidence.architect_chatgpt
    writer_gemini = [bool]$providerEvidence.writer_gemini
}
$boundaryDiagnosticPath = Join-Path $EvidenceDir 'd08-boundary-diagnostic.json'
[System.IO.File]::WriteAllText($boundaryDiagnosticPath, (($boundaryDiagnostic | ConvertTo-Json -Depth 4) + "`r`n"), (New-Object System.Text.UTF8Encoding($false)))
$completedText = if ($completed.Count -eq 0) { '<empty>' } else { ($completed -join ',') }
$boundarySummary = "completed=$completedText phase=$($boundaryDiagnostic.phase) current=$($boundaryDiagnostic.current_chapter) in_progress=$($boundaryDiagnostic.in_progress_chapter) total=$($boundaryDiagnostic.total_chapters) chapter_files=$($boundaryDiagnostic.chapter_file_count) architect_files=$($boundaryDiagnostic.architect_session_files) writer_files=$($boundaryDiagnostic.writer_session_files) architect_chatgpt=$($boundaryDiagnostic.architect_chatgpt) writer_gemini=$($boundaryDiagnostic.writer_gemini)"
Write-Host "D08 sanitized boundary diagnostic: $boundarySummary"

if ($completed -notcontains 1 -or $completed -contains 2) { Fail ("packaged run did not stop at exact one-chapter boundary; " + $boundarySummary) }
if ($chapters.Count -ne 1 -or $chapters[0].Length -lt 100) { Fail ("expected exactly one non-trivial committed chapter; " + $boundarySummary) }

if (-not [bool]$providerEvidence.architect_chatgpt) { Fail 'no architect assistant session entry proved ChatGPT specialist execution' }
if (-not [bool]$providerEvidence.writer_gemini) { Fail 'no writer assistant session entry proved Gemini primary execution' }

$runtimeText = ''
foreach ($path in @($stdoutPath, $stderrPath, (Join-Path $outputRoot 'headless.log'))) {
    if (Test-Path -LiteralPath $path) { $runtimeText += "`n" + (Get-Content -LiteralPath $path -Raw -Encoding UTF8) }
}
if ($runtimeText -notmatch 'default=web/gemini-web' -or $runtimeText -notmatch 'specialist=chatgpt-web/chatgpt-web') {
    Fail 'strict-role model graph was not projected by packaged runtime'
}
$forbidden = @(
    ('OPENAI_' + 'API_KEY'),
    ('ANTHROPIC_' + 'API_KEY'),
    ('GEMINI_' + 'API_KEY'),
    ('OPENROUTER_' + 'API_KEY'),
    ('api.' + 'openai.com'),
    ('api.' + 'anthropic.com'),
    ('openrouter.' + 'ai/api'),
    ('generativelanguage.' + 'googleapis.com'),
    ('localhost:' + '11434')
)
foreach ($marker in $forbidden) {
    if ($runtimeText.IndexOf($marker, [StringComparison]::OrdinalIgnoreCase) -ge 0) { Fail "forbidden API/runtime marker observed: $marker" }
}

$evidence = [ordered]@{
    schema = 'ainovel-d08-dual-provider-packaged-smoke/1'
    git_sha = $ExpectedGitSha
    archive_name = [IO.Path]::GetFileName($PackageArchivePath)
    archive_sha256 = $archiveSha
    packaged_commit_verified = $true
    gemini_ready = $true
    gemini_restart_ready = $true
    chatgpt_ready = $true
    chatgpt_restart_ready = $true
    strict_role_architect_provider = 'chatgpt-web'
    strict_role_writer_provider = 'web'
    strict_role_writer_model = 'gemini-web'
    architect_session_files = [int]$providerEvidence.architect_files
    writer_session_files = [int]$providerEvidence.writer_files
    completed_chapters = $completed
    normal_shutdown_gemini_clean = $true
    normal_shutdown_chatgpt_clean = $true
    static_no_api = 'PASS'
    runtime_no_api = 'PASS'
    verified_at = (Get-Date).ToUniversalTime().ToString('o')
}
$evidencePath = Join-Path $EvidenceDir 'd08-dual-provider-packaged-smoke-evidence.json'
[System.IO.File]::WriteAllText($evidencePath, (($evidence | ConvertTo-Json -Depth 6) + "`r`n"), (New-Object System.Text.UTF8Encoding($false)))
Write-Host 'D08 DUAL-PROVIDER PACKAGED WEB-ONLY SMOKE: PASS'

# Cleanup is not evidence: all assertions above require normal shutdown first.
Stop-ProfileChrome $geminiProfileDir
Stop-ProfileChrome $chatgptProfileDir