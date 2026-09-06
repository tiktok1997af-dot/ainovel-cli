[CmdletBinding()]
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")),
    [string]$EvidenceDir = "",
    [int]$ReadinessTimeoutSeconds = 45,
    [int]$FirstChapterTimeoutSeconds = 1200,
    [int]$ResumeTimeoutSeconds = 1200
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "W5E production smoke FAIL: $Message"
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

if (-not $IsWindows) { Fail "this gate requires a real Windows desktop with visible Google Chrome" }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Fail "go is required" }
if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Fail "git is required" }

$RepoRoot = (Resolve-Path $RepoRoot).Path
if ([string]::IsNullOrWhiteSpace($EvidenceDir)) {
    $EvidenceDir = Join-Path $env:TEMP ("ainovel-w5e-evidence-" + [guid]::NewGuid().ToString("N"))
}
$EvidenceDir = [IO.Path]::GetFullPath($EvidenceDir)
New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null

$workspace = Join-Path $env:TEMP ("ainovel-w5e-workspace-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $workspace | Out-Null
$distDir = Join-Path $EvidenceDir "bin"
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

$productionExe = Join-Path $distDir "ainovel-cli.exe"
$readinessExe = Join-Path $distDir "ainovel-w5e-readiness.exe"
$readinessEvidence = Join-Path $EvidenceDir "w5e-readiness-evidence.json"
$firstOut = Join-Path $EvidenceDir "first.stdout.log"
$firstErr = Join-Path $EvidenceDir "first.stderr.log"
$resumeOut = Join-Path $EvidenceDir "resume.stdout.log"
$resumeErr = Join-Path $EvidenceDir "resume.stderr.log"

Push-Location $RepoRoot
try {
    $env:GOWORK = "off"
    $gitSha = (git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { Fail "cannot resolve git HEAD" }

    & go build -trimpath -buildvcs=true -o $productionExe ./cmd/ainovel-cli
    if ($LASTEXITCODE -ne 0) { Fail "production binary build failed" }
    & go build -trimpath -buildvcs=true -o $readinessExe ./cmd/ainovel-w5e-readiness
    if ($LASTEXITCODE -ne 0) { Fail "readiness verifier build failed" }

    & bash scripts/w55-no-api-audit.sh
    if ($LASTEXITCODE -ne 0) { Fail "W5.5 NO-API audit failed on the candidate head" }
} finally {
    Pop-Location
}

$productionHash = (Get-FileHash -LiteralPath $productionExe -Algorithm SHA256).Hash.ToLowerInvariant()

Push-Location $workspace
try {
    & $readinessExe -timeout ("{0}s" -f $ReadinessTimeoutSeconds) -evidence $readinessEvidence
    if ($LASTEXITCODE -ne 0) { Fail "Gemini Web READY/restart READY verification failed" }
} finally {
    Pop-Location
}

$ready = Get-Content -LiteralPath $readinessEvidence -Raw -Encoding UTF8 | ConvertFrom-Json
if (-not $ready.first_ready -or -not $ready.restart_ready) { Fail "sanitized readiness evidence is incomplete" }
if ($ready.site -ne "gemini-web") { Fail ("unexpected WEB site identity: " + $ready.site) }
$profileName = [string]$ready.profile_name
if ([string]::IsNullOrWhiteSpace($profileName)) { $profileName = "default" }
$profileDir = Join-Path ([Environment]::GetFolderPath("UserProfile")) (".ainovel\browser\profiles\" + $profileName)

$promptPath = Join-Path $workspace "w5e-prompt.txt"
@"
Hãy tạo một truyện ngắn thử nghiệm bằng tiếng Việt, chính xác 2 chương và không được mở chương thứ 3. Mục tiêu của bài thử là kiểm chứng toàn bộ dây chuyền sản xuất: phải lập nền tảng truyện, lập kế hoạch, viết chính văn, dùng các công cụ cục bộ để lưu dữ liệu và hoàn tất đúng hai chương. Nội dung đơn giản: một người giao thư trong thành phố mưa phát hiện bức thư cuối cùng được gửi cho chính mình. Mỗi chương cần có diễn biến rõ ràng, kết thúc truyện ở chương 2. Không giải thích quy trình cho người dùng; hãy thực thi quy trình sáng tác bình thường của hệ thống.
"@ | Set-Content -LiteralPath $promptPath -Encoding UTF8

$progressPath = Join-Path $workspace "output\novel\meta\progress.json"
$first = Start-Process -FilePath $productionExe -ArgumentList @("--headless", "--prompt-file", $promptPath) -WorkingDirectory $workspace -RedirectStandardOutput $firstOut -RedirectStandardError $firstErr -PassThru
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
            $chapterFiles = @(Get-ChildItem -LiteralPath (Join-Path $workspace "output\novel\chapters") -Filter "*.md" -File -ErrorAction SilentlyContinue | Sort-Object Name)
            if ($chapterFiles.Count -ge 1) {
                $checkpoint = $progress
                break
            }
        }
        if ($first.HasExited) {
            try { $first.WaitForExit() } catch { }
            try { $first.Refresh() } catch { }
            $exitCode = $first.ExitCode
            $stderrDiagnostic = Get-SanitizedDiagnosticTail $firstErr
            Fail ("first production process exited before a resumable chapter-1 boundary; exit=" + $exitCode + "; stderr=" + $stderrDiagnostic)
        }
    }
    if ($null -eq $checkpoint) { Fail "timed out waiting for a durable, resumable chapter-1 boundary" }
} finally {
    if (-not $first.HasExited) { Stop-Process -Id $first.Id -Force -ErrorAction SilentlyContinue }
    try { $first.WaitForExit(10000) | Out-Null } catch { }
    Stop-SmokeChrome $profileDir
    Start-Sleep -Seconds 2
}

$chapterOneFile = @(Get-ChildItem -LiteralPath (Join-Path $workspace "output\novel\chapters") -Filter "*.md" -File | Sort-Object Name)[0]
$chapterOneHashBefore = (Get-FileHash -LiteralPath $chapterOneFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
$completedBefore = @($checkpoint.completed_chapters | ForEach-Object { [int]$_ })

$resume = Start-Process -FilePath $productionExe -ArgumentList @("--headless") -WorkingDirectory $workspace -RedirectStandardOutput $resumeOut -RedirectStandardError $resumeErr -PassThru
if (-not $resume.WaitForExit($ResumeTimeoutSeconds * 1000)) {
    Stop-Process -Id $resume.Id -Force -ErrorAction SilentlyContinue
    Stop-SmokeChrome $profileDir
    Fail "resume process timed out"
}
Stop-SmokeChrome $profileDir
if ($resume.ExitCode -ne 0) { Fail ("resume production process failed; exit=" + $resume.ExitCode + "; stderr=" + (Get-SanitizedDiagnosticTail $resumeErr)) }

$finalProgress = Get-Progress $progressPath
if ($null -eq $finalProgress) { Fail "progress.json missing after restart" }
if (-not (Has-Chapter $finalProgress 1)) { Fail "chapter 1 disappeared after restart" }
if (-not (Has-Chapter $finalProgress 2)) { Fail "chapter 2 was not completed after restart" }

$chapterOneHashAfter = (Get-FileHash -LiteralPath $chapterOneFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
if ($chapterOneHashBefore -ne $chapterOneHashAfter) { Fail "chapter 1 changed across restart" }

$resumeText = Read-TextFiles @($resumeOut, $resumeErr)
if ($resumeText -notmatch "headless\s+恢復|headless\s+恢复") { Fail "second production invocation did not report the real Resume path" }

$sessionDir = Join-Path $workspace "output\novel\meta\sessions\agents"
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
    (Join-Path $workspace "output\novel\headless.log")
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
    schema = "ainovel-w5e-production-smoke/1"
    git_sha = $gitSha
    production_binary_sha256 = $productionHash
    web_site = [string]$ready.site
    profile_name = $profileName
    first_ready = [bool]$ready.first_ready
    restart_ready = [bool]$ready.restart_ready
    first_boundary = [ordered]@{
        phase = [string]$checkpoint.phase
        current_chapter = [int]$checkpoint.current_chapter
        completed_chapters = $completedBefore
    }
    restart_resume_detected = $true
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
    verified_at = (Get-Date).ToUniversalTime().ToString("o")
}
$evidencePath = Join-Path $EvidenceDir "w5e-production-smoke-evidence.json"
$evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $evidencePath -Encoding UTF8

Write-Host "W5E REAL PRODUCTION APP SMOKE: PASS"
Write-Host ("Evidence: " + $evidencePath)
Write-Host ("Candidate: " + $gitSha)
Write-Host ("Production SHA256: " + $productionHash)
