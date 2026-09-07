[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RepoRoot,
    [Parameter(Mandatory = $true)][string]$PackageArchivePath,
    [Parameter(Mandatory = $true)][string]$ChecksumManifestPath,
    [Parameter(Mandatory = $true)][string]$ReadinessExe,
    [Parameter(Mandatory = $true)][string]$ExpectedReadinessSha256,
    [Parameter(Mandatory = $true)][string]$ExpectedGitSha,
    [Parameter(Mandatory = $true)][string]$ExpectedSnapshotVersion,
    [Parameter(Mandatory = $true)][string]$EvidenceDir,
    [Parameter(Mandatory = $true)][string]$RuntimeDir,
    [int]$ReadinessTimeoutSeconds = 60,
    [int]$FirstChapterTimeoutSeconds = 1200,
    [int]$ResumeTimeoutSeconds = 1200
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "W6C release-artifact smoke FAIL: $Message"
}

function Get-Progress([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    try {
        return Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        return $null
    }
}

function Has-Chapter($Progress, [int]$Chapter) {
    if ($null -eq $Progress -or $null -eq $Progress.completed_chapters) { return $false }
    return @($Progress.completed_chapters) -contains $Chapter
}

function Stop-SmokeChrome([string]$ProfileDir) {
    if ([string]::IsNullOrWhiteSpace($ProfileDir)) { return }
    $needle = "--user-data-dir=$ProfileDir"
    Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -and $_.CommandLine.Contains($needle) } |
        ForEach-Object {
            try { Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop } catch { }
        }
}

function Stop-SmokeProduction([string]$ExecutablePath) {
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

function Read-TextFiles([string[]]$Paths) {
    $parts = New-Object System.Collections.Generic.List[string]
    foreach ($path in $Paths) {
        if (Test-Path -LiteralPath $path) {
            $parts.Add((Get-Content -LiteralPath $path -Raw -Encoding UTF8))
        }
    }
    return ($parts -join "`n")
}

function Get-SanitizedDiagnosticTail([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return "<stderr-missing>" }

    $diagnosticPattern = "(?i)(error|fail|fatal|panic|auth|required|config|browser|chrome|web|headless|host|engine|invalid|missing|not found|timeout|exit|cannot|unable|ready|denied|refused|unexpected)"
    $lines = @(Get-Content -LiteralPath $Path -Encoding UTF8 -Tail 80 -ErrorAction SilentlyContinue)
    $safe = New-Object System.Collections.Generic.List[string]
    $userHome = [Environment]::GetFolderPath("UserProfile")

    foreach ($raw in $lines) {
        $line = [string]$raw
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        if ($line -notmatch $diagnosticPattern) { continue }
        if (-not [string]::IsNullOrWhiteSpace($userHome)) {
            $line = $line.Replace($userHome, "<USERPROFILE>")
        }
        $line = [regex]::Replace($line, "(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b", "<redacted-email>")
        $line = [regex]::Replace($line, "(?i)\b(cookie|authorization|bearer|token|password|secret|api[_-]?key)\b\s*[:=]\s*[^\s,;]+", '$1=<redacted>')
        $line = [regex]::Replace($line, "(?i)(https?://[^\s?#]+)(?:\?[^\s#]*)?(?:#[^\s]*)?", '$1')
        if ($line.Length -gt 600) { $line = $line.Substring(0, 600) + "<truncated>" }
        $safe.Add($line)
    }

    if ($safe.Count -eq 0) { return "<no-sanitized-diagnostic-lines>" }
    return (($safe | Select-Object -Last 12) -join " | ")
}

function Verify-PackageChecksum([string]$ArchivePath, [string]$ManifestPath, [string]$SnapshotVersion) {
    $archiveName = [IO.Path]::GetFileName($ArchivePath)
    $expectedName = "ainovel-cli_${SnapshotVersion}_Windows_x86_64.zip"
    if ($archiveName -ne $expectedName) {
        Fail ("unexpected Windows x86_64 archive name: got " + $archiveName + ", expected " + $expectedName)
    }

    $matches = New-Object System.Collections.Generic.List[string]
    foreach ($raw in Get-Content -LiteralPath $ManifestPath -Encoding UTF8) {
        $line = ([string]$raw).Trim()
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $fields = @($line -split "\s+" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if ($fields.Count -ne 2) { continue }
        $name = ([string]$fields[1]).TrimStart('*')
        if ($name -eq $archiveName) {
            $matches.Add(([string]$fields[0]).ToLowerInvariant())
        }
    }
    if ($matches.Count -ne 1) {
        Fail ("checksum manifest must contain exactly one entry for " + $archiveName + "; found " + $matches.Count)
    }
    $expected = $matches[0]
    if ($expected -notmatch '^[0-9a-f]{64}$') {
        Fail ("invalid SHA-256 entry for " + $archiveName)
    }
    $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Fail ("release archive SHA-256 mismatch: got " + $actual + ", expected " + $expected)
    }
    return $actual
}

if ($env:OS -ne "Windows_NT") { Fail "this gate requires a real Windows desktop with visible Google Chrome" }
foreach ($required in @("git", "bash")) {
    if (-not (Get-Command $required -ErrorAction SilentlyContinue)) { Fail ("required command unavailable: " + $required) }
}

$RepoRoot = (Resolve-Path $RepoRoot).Path
$PackageArchivePath = (Resolve-Path $PackageArchivePath).Path
$ChecksumManifestPath = (Resolve-Path $ChecksumManifestPath).Path
$ReadinessExe = (Resolve-Path $ReadinessExe).Path
$EvidenceDir = [IO.Path]::GetFullPath($EvidenceDir)
$RuntimeDir = [IO.Path]::GetFullPath($RuntimeDir)
$ExpectedGitSha = $ExpectedGitSha.Trim().ToLowerInvariant()
$ExpectedSnapshotVersion = $ExpectedSnapshotVersion.Trim()
$ExpectedReadinessSha256 = $ExpectedReadinessSha256.Trim().ToLowerInvariant()

if ($ExpectedGitSha -notmatch '^[0-9a-f]{40}$') { Fail "expected candidate SHA must be a full lowercase Git SHA" }
if ($ExpectedReadinessSha256 -notmatch '^[0-9a-f]{64}$') { Fail "expected readiness helper SHA-256 is invalid" }
if ([string]::IsNullOrWhiteSpace($ExpectedSnapshotVersion)) { Fail "expected snapshot version is empty" }

New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null
if (Test-Path -LiteralPath $RuntimeDir) { Remove-Item -LiteralPath $RuntimeDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $RuntimeDir | Out-Null

$gitSha = ""
Push-Location $RepoRoot
try {
    $gitSha = (git rev-parse HEAD).Trim().ToLowerInvariant()
    if ($LASTEXITCODE -ne 0) { Fail "cannot resolve git HEAD" }
    if ($gitSha -ne $ExpectedGitSha) { Fail ("candidate drift: got " + $gitSha + ", expected " + $ExpectedGitSha) }

    & bash scripts/w55-no-api-audit.sh
    if ($LASTEXITCODE -ne 0) { Fail "W5.5 NO-API audit failed on the exact candidate head" }
} finally {
    Pop-Location
}

$packageArchiveHash = Verify-PackageChecksum $PackageArchivePath $ChecksumManifestPath $ExpectedSnapshotVersion
$readinessHash = (Get-FileHash -LiteralPath $ReadinessExe -Algorithm SHA256).Hash.ToLowerInvariant()
if ($readinessHash -ne $ExpectedReadinessSha256) {
    Fail ("readiness helper SHA-256 mismatch: got " + $readinessHash + ", expected " + $ExpectedReadinessSha256)
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead($PackageArchivePath)
try {
    $fileNames = @($zip.Entries | Where-Object { -not [string]::IsNullOrWhiteSpace($_.Name) } | ForEach-Object { ([string]$_.FullName).Replace('\', '/') } | Sort-Object)
    $expectedFiles = @("LICENSE", "README.md", "ainovel-cli.exe") | Sort-Object
    if (($fileNames -join "|") -ne ($expectedFiles -join "|")) {
        Fail ("release archive contents mismatch: " + ($fileNames -join ", "))
    }
    $rootExecutables = @($zip.Entries | Where-Object { ([string]$_.FullName).Replace('\', '/') -eq "ainovel-cli.exe" })
    if ($rootExecutables.Count -ne 1) { Fail "release archive must contain exactly one root-level ainovel-cli.exe" }
} finally {
    $zip.Dispose()
}

$packageDir = Join-Path $RuntimeDir "package"
New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
[System.IO.Compression.ZipFile]::ExtractToDirectory($PackageArchivePath, $packageDir)
$productionExe = Join-Path $packageDir "ainovel-cli.exe"
if (-not (Test-Path -LiteralPath $productionExe)) { Fail "packaged ainovel-cli.exe was not extracted" }
$productionHash = (Get-FileHash -LiteralPath $productionExe -Algorithm SHA256).Hash.ToLowerInvariant()

$versionOutput = @(& $productionExe --version 2>&1 | ForEach-Object { [string]$_ })
if ($LASTEXITCODE -ne 0) { Fail "packaged production binary --version failed" }
$versionLines = @($versionOutput | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
$expectedDisplayVersion = if ($ExpectedSnapshotVersion.StartsWith("v")) { $ExpectedSnapshotVersion } else { "v" + $ExpectedSnapshotVersion }
if ($versionLines.Count -lt 3 -or $versionLines[0] -ne ("ainovel-cli " + $expectedDisplayVersion)) {
    Fail ("packaged production binary reported unexpected version metadata")
}
if ($versionLines -notcontains ("commit: " + $ExpectedGitSha)) {
    Fail "packaged production binary embedded commit does not match exact candidate"
}

$readinessEvidence = Join-Path $EvidenceDir "w6c-readiness-evidence.json"
$firstOut = Join-Path $EvidenceDir "first.stdout.log"
$firstErr = Join-Path $EvidenceDir "first.stderr.log"
$resumeOut = Join-Path $EvidenceDir "resume.stdout.log"
$resumeErr = Join-Path $EvidenceDir "resume.stderr.log"
$resumeExitCodePath = Join-Path $EvidenceDir "resume.exitcode.txt"
$resumeWrapperPath = Join-Path $RuntimeDir "w6c-resume-wrapper.cmd"

Push-Location $RuntimeDir
try {
    & $ReadinessExe -timeout ("{0}s" -f $ReadinessTimeoutSeconds) -evidence $readinessEvidence
    if ($LASTEXITCODE -ne 0) { Fail "Gemini Web READY/restart READY verification failed" }
} finally {
    Pop-Location
}

$ready = Get-Content -LiteralPath $readinessEvidence -Raw -Encoding UTF8 | ConvertFrom-Json
if (-not $ready.first_ready -or -not $ready.restart_ready) { Fail "sanitized readiness evidence is incomplete" }
if ([string]$ready.site -ne "gemini-web") { Fail ("unexpected WEB site identity: " + [string]$ready.site) }
$profileName = [string]$ready.profile_name
if ([string]::IsNullOrWhiteSpace($profileName)) { $profileName = "default" }
$profileDir = Join-Path ([Environment]::GetFolderPath("UserProfile")) (".ainovel\browser\profiles\" + $profileName)

$promptPath = Join-Path $RuntimeDir "w6c-prompt.txt"
@"
Hãy tạo một truyện ngắn thử nghiệm bằng tiếng Việt, chính xác 2 chương và không được mở chương thứ 3. Mục tiêu của bài thử là kiểm chứng toàn bộ dây chuyền sản xuất: phải lập nền tảng truyện, lập kế hoạch, viết chính văn, dùng các công cụ cục bộ để lưu dữ liệu và hoàn tất đúng hai chương. Nội dung đơn giản: một người giao thư trong thành phố mưa phát hiện bức thư cuối cùng được gửi cho chính mình. Mỗi chương cần có diễn biến rõ ràng, kết thúc truyện ở chương 2. Không giải thích quy trình cho người dùng; hãy thực thi quy trình sáng tác bình thường của hệ thống.
"@ | Set-Content -LiteralPath $promptPath -Encoding UTF8

$progressPath = Join-Path $RuntimeDir "output\novel\meta\progress.json"
$first = Start-Process -FilePath $productionExe -ArgumentList @("--headless", "--prompt-file", $promptPath) -WorkingDirectory $RuntimeDir -RedirectStandardOutput $firstOut -RedirectStandardError $firstErr -PassThru
$deadline = (Get-Date).AddSeconds($FirstChapterTimeoutSeconds)
$checkpoint = $null
try {
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 500
        $progress = Get-Progress $progressPath
        if (Has-Chapter $progress 2) {
            Fail "chapter 2 completed before the runner could create the required restart boundary after chapter 1"
        }
        if ((Has-Chapter $progress 1) -and ([string]$progress.phase -eq "writing") -and ([int]$progress.current_chapter -gt 0)) {
            $chapterFiles = @(Get-ChildItem -LiteralPath (Join-Path $RuntimeDir "output\novel\chapters") -Filter "*.md" -File -ErrorAction SilentlyContinue | Sort-Object Name)
            if ($chapterFiles.Count -ge 1) {
                $checkpoint = $progress
                break
            }
        }
        if ($first.HasExited) {
            try { $first.WaitForExit() } catch { }
            try { $first.Refresh() } catch { }
            $stderrDiagnostic = Get-SanitizedDiagnosticTail $firstErr
            Fail ("first packaged production process exited before a resumable chapter-1 boundary; stderr=" + $stderrDiagnostic)
        }
    }
    if ($null -eq $checkpoint) { Fail "timed out waiting for a durable, resumable chapter-1 boundary" }
} finally {
    if (-not $first.HasExited) { Stop-Process -Id $first.Id -Force -ErrorAction SilentlyContinue }
    try { $first.WaitForExit(10000) | Out-Null } catch { }
    Stop-SmokeChrome $profileDir
    Start-Sleep -Seconds 2
}

$chapterOneFile = @(Get-ChildItem -LiteralPath (Join-Path $RuntimeDir "output\novel\chapters") -Filter "*.md" -File | Sort-Object Name)[0]
$chapterOneHashBefore = (Get-FileHash -LiteralPath $chapterOneFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
$completedBefore = @($checkpoint.completed_chapters | ForEach-Object { [int]$_ })
if ($completedBefore -notcontains 1 -or $completedBefore -contains 2) { Fail "chapter-1 checkpoint is not a valid resume source" }
$progressHashBeforeRestart = (Get-FileHash -LiteralPath $progressPath -Algorithm SHA256).Hash.ToLowerInvariant()

$cmdLines = @(
    '@echo off',
    '"%W6C_RESUME_PRODUCTION_EXE%" --headless',
    'set "W6C_RESUME_EXIT_CODE=%ERRORLEVEL%"',
    '> "%W6C_RESUME_EXIT_CODE_PATH%" echo %W6C_RESUME_EXIT_CODE%',
    'exit /b %W6C_RESUME_EXIT_CODE%'
)
[System.IO.File]::WriteAllLines($resumeWrapperPath, $cmdLines, [System.Text.Encoding]::ASCII)

Remove-Item -LiteralPath $resumeExitCodePath -Force -ErrorAction SilentlyContinue
$env:W6C_RESUME_PRODUCTION_EXE = $productionExe
$env:W6C_RESUME_EXIT_CODE_PATH = $resumeExitCodePath
$cmdExe = [string]$env:ComSpec
if ([string]::IsNullOrWhiteSpace($cmdExe) -or -not (Test-Path -LiteralPath $cmdExe)) {
    Remove-Item Env:W6C_RESUME_PRODUCTION_EXE -ErrorAction SilentlyContinue
    Remove-Item Env:W6C_RESUME_EXIT_CODE_PATH -ErrorAction SilentlyContinue
    Fail "cmd.exe is required for byte-preserving resume output"
}
$resume = Start-Process -FilePath $cmdExe -ArgumentList @("/d", "/s", "/c", ('"' + $resumeWrapperPath + '"')) -WorkingDirectory $RuntimeDir -RedirectStandardOutput $resumeOut -RedirectStandardError $resumeErr -PassThru
Remove-Item Env:W6C_RESUME_PRODUCTION_EXE -ErrorAction SilentlyContinue
Remove-Item Env:W6C_RESUME_EXIT_CODE_PATH -ErrorAction SilentlyContinue

if (-not $resume.WaitForExit($ResumeTimeoutSeconds * 1000)) {
    Stop-Process -Id $resume.Id -Force -ErrorAction SilentlyContinue
    Stop-SmokeProduction $productionExe
    Stop-SmokeChrome $profileDir
    Fail "resume process timed out"
}
try { $resume.WaitForExit() } catch { }
Stop-SmokeChrome $profileDir

if (-not (Test-Path -LiteralPath $resumeExitCodePath)) {
    Stop-SmokeProduction $productionExe
    Fail "resume exit-code sidecar missing after process completion"
}
$resumeExitRaw = (Get-Content -LiteralPath $resumeExitCodePath -Raw -Encoding UTF8).Trim()
$resumeExitCode = 0
if (-not [int]::TryParse($resumeExitRaw, [ref]$resumeExitCode)) {
    Stop-SmokeProduction $productionExe
    Fail ("resume exit-code sidecar is invalid: " + $resumeExitRaw)
}
if ($resumeExitCode -ne 0) {
    Fail ("resume packaged production process failed; exit=" + $resumeExitCode + "; stderr=" + (Get-SanitizedDiagnosticTail $resumeErr))
}

$finalProgress = Get-Progress $progressPath
if ($null -eq $finalProgress) { Fail "progress.json missing after restart" }
if (-not (Has-Chapter $finalProgress 1)) { Fail "chapter 1 disappeared after restart" }
if (-not (Has-Chapter $finalProgress 2)) { Fail "chapter 2 was not completed after restart" }

$chapterOneHashAfter = (Get-FileHash -LiteralPath $chapterOneFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($chapterOneHashBefore -ne $chapterOneHashAfter) { Fail "chapter 1 changed across restart" }
$progressHashAfterResume = (Get-FileHash -LiteralPath $progressPath -Algorithm SHA256).Hash.ToLowerInvariant()
$resumeStateAdvanced = $progressHashBeforeRestart -ne $progressHashAfterResume
if (-not $resumeStateAdvanced) { Fail "second packaged production invocation did not report the real Resume path" }

$sessionDir = Join-Path $RuntimeDir "output\novel\meta\sessions\agents"
$sessionFiles = @(Get-ChildItem -LiteralPath $sessionDir -Filter "*.jsonl" -File -ErrorAction SilentlyContinue)
if ($sessionFiles.Count -eq 0) { Fail "no real Worker session logs were persisted" }
$sessionText = Read-TextFiles @($sessionFiles.FullName)
$requiredTools = @("draft_chapter", "commit_chapter")
$observedTools = New-Object System.Collections.Generic.List[string]
foreach ($tool in $requiredTools) {
    if ($sessionText -notmatch [regex]::Escape($tool)) { Fail ("required local tool evidence missing: " + $tool) }
    $observedTools.Add($tool)
}
foreach ($optional in @("save_foundation", "save_book", "plan_chapter", "save_review")) {
    if ($sessionText -match [regex]::Escape($optional)) { $observedTools.Add($optional) }
}

$runtimeText = Read-TextFiles @(
    $firstOut, $firstErr, $resumeOut, $resumeErr,
    (Join-Path $RuntimeDir "output\novel\headless.log")
)
$forbiddenRuntimeMarkers = @(
    ("OPENAI_" + "API_KEY"),
    ("ANTHROPIC_" + "API_KEY"),
    ("GEMINI_" + "API_KEY"),
    ("OPENROUTER_" + "API_KEY"),
    ("api." + "openai.com"),
    ("api." + "anthropic.com"),
    ("openrouter." + "ai/api"),
    ("generativelanguage." + "googleapis.com"),
    ("localhost:" + "11434"),
    ("olla" + "ma")
)
foreach ($marker in $forbiddenRuntimeMarkers) {
    if ($runtimeText.IndexOf($marker, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
        Fail ("forbidden API/fallback/local-provider runtime marker observed: " + $marker)
    }
}

$finalChapters = @($finalProgress.completed_chapters | ForEach-Object { [int]$_ })
$evidence = [ordered]@{
    schema = "ainovel-w6c-release-artifact-smoke/1"
    git_sha = $gitSha
    binary_source = "github-actions-goreleaser-snapshot-artifact"
    desktop_build_performed = $false
    package_archive_name = [IO.Path]::GetFileName($PackageArchivePath)
    package_archive_sha256 = $packageArchiveHash
    package_checksum_verified = $true
    package_snapshot_version = $ExpectedSnapshotVersion
    production_binary_sha256 = $productionHash
    production_binary_commit = $ExpectedGitSha
    production_binary_commit_verified = $true
    readiness_helper_sha256 = $readinessHash
    web_site = [string]$ready.site
    profile_name = $profileName
    first_ready = [bool]$ready.first_ready
    restart_ready = [bool]$ready.restart_ready
    first_boundary = [ordered]@{
        phase = [string]$checkpoint.phase
        current_chapter = [int]$checkpoint.current_chapter
        completed_chapters = $completedBefore
    }
    resume_invocation_mode = "headless-existing-state"
    resume_progress_sha256_before_restart = $progressHashBeforeRestart
    resume_progress_sha256_after_resume = $progressHashAfterResume
    restart_resume_detected = [bool]$resumeStateAdvanced
    final_progress = [ordered]@{
        phase = [string]$finalProgress.phase
        current_chapter = [int]$finalProgress.current_chapter
        completed_chapters = $finalChapters
    }
    chapter_1_sha256_before_restart = $chapterOneHashBefore
    chapter_1_sha256_after_restart = $chapterOneHashAfter
    session_file_count = $sessionFiles.Count
    observed_local_tools = @($observedTools)
    runtime_no_api_marker_scan = "PASS"
    w55_static_no_api_gate = "PASS"
    no_upstream_fallback = $true
    verified_at = (Get-Date).ToUniversalTime().ToString("o")
}
$evidencePath = Join-Path $EvidenceDir "w6c-release-artifact-smoke-evidence.json"
$evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $evidencePath -Encoding UTF8

Write-Host "W6C RELEASE-ARTIFACT WEB-ONLY DESKTOP SMOKE: PASS"
Write-Host ("Evidence: " + $evidencePath)
Write-Host ("Candidate: " + $gitSha)
Write-Host ("Package SHA256: " + $packageArchiveHash)
Write-Host ("Production SHA256: " + $productionHash)
