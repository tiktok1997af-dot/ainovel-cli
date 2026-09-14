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
            if ($null -eq $entry._meta) { continue }
            $provider = ([string]$entry._meta.provider).Trim().ToLowerInvariant()
            $model = ([string]$entry._meta.model).Trim().ToLowerInvariant()
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
foreach ($required in @('git','bash')) {
    if (-not (Get-Command $required -ErrorAction SilentlyContinue)) { Fail "required command unavailable: $required" }
}

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
    & bash scripts/w55-no-api-audit.sh
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
$process = Start-Process -FilePath $productionExe -ArgumentList @('--headless','--prompt-file',$promptPath) -WorkingDirectory $RuntimeDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -PassThru
try {
    if (-not $process.WaitForExit($RunTimeoutSeconds * 1000)) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        Fail 'packaged strict-role run timed out'
    }
    try { $process.WaitForExit() } catch { }
    if ($process.ExitCode -ne 0) { Fail "packaged strict-role run exited with code $($process.ExitCode)" }
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
if ($completed -notcontains 1 -or $completed -contains 2) { Fail 'packaged run did not stop at exact one-chapter boundary' }

$chapters = @(Get-ChildItem -LiteralPath (Join-Path $outputRoot 'chapters') -Filter '*.md' -File -ErrorAction SilentlyContinue)
if ($chapters.Count -ne 1 -or $chapters[0].Length -lt 100) { Fail 'expected exactly one non-trivial committed chapter' }

$providerEvidence = Read-SessionProviderEvidence (Join-Path $outputRoot 'meta\sessions\agents')
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
