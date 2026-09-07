[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RepoRoot,
    [Parameter(Mandatory = $true)][string]$PackageArchivePath,
    [Parameter(Mandatory = $true)][string]$ChecksumManifestPath,
    [Parameter(Mandatory = $true)][string]$ReadinessExe,
    [Parameter(Mandatory = $true)][string]$ExpectedReadinessSha256,
    [Parameter(Mandatory = $true)][string]$ExpectedGitSha,
    [Parameter(Mandatory = $true)][string]$ReleaseTag,
    [Parameter(Mandatory = $true)][string]$EvidenceDir,
    [Parameter(Mandatory = $true)][string]$RuntimeDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Fail([string]$Message) { throw "W6D published Windows smoke FAIL: $Message" }

$ReleaseTag = $ReleaseTag.Trim()
$ExpectedGitSha = $ExpectedGitSha.Trim().ToLowerInvariant()
if ($ReleaseTag -notmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { Fail "release tag is not strict stable SemVer" }
if ($ExpectedGitSha -notmatch '^[0-9a-f]{40}$') { Fail "expected release SHA is invalid" }
$version = $ReleaseTag.Substring(1)
$archiveName = [IO.Path]::GetFileName($PackageArchivePath)
$expectedArchive = "ainovel-cli_${version}_Windows_x86_64.zip"
if ($archiveName -ne $expectedArchive) { Fail "unexpected published Windows archive name: $archiveName" }

$expectedUrl = "https://github.com/tiktok1997af-dot/ainovel-cli/releases/download/$ReleaseTag/$expectedArchive"
$checksumUrl = "https://github.com/tiktok1997af-dot/ainovel-cli/releases/download/$ReleaseTag/ainovel-cli_checksums.txt"

New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null
& (Join-Path $RepoRoot 'scripts/w6c-release-artifact-smoke.ps1') `
    -RepoRoot $RepoRoot `
    -PackageArchivePath $PackageArchivePath `
    -ChecksumManifestPath $ChecksumManifestPath `
    -ReadinessExe $ReadinessExe `
    -ExpectedReadinessSha256 $ExpectedReadinessSha256 `
    -ExpectedGitSha $ExpectedGitSha `
    -ExpectedSnapshotVersion $version `
    -EvidenceDir $EvidenceDir `
    -RuntimeDir $RuntimeDir `
    -ReadinessTimeoutSeconds 60 `
    -FirstChapterTimeoutSeconds 1200 `
    -ResumeTimeoutSeconds 1200
if ($LASTEXITCODE -ne 0) { Fail "underlying packaged production authority returned non-zero" }

$witnessPath = Join-Path $EvidenceDir 'w6c-release-artifact-smoke-evidence.json'
if (-not (Test-Path -LiteralPath $witnessPath)) { Fail "missing underlying execution witness" }
$witness = Get-Content -LiteralPath $witnessPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ([string]$witness.schema -ne 'ainovel-w6c-release-artifact-smoke/1') { Fail "unexpected execution witness schema" }
if ([string]$witness.git_sha -ne $ExpectedGitSha) { Fail "published binary commit witness mismatch" }
if ([bool]$witness.desktop_build_performed) { Fail "desktop rebuilt production binary" }
if (-not [bool]$witness.package_checksum_verified -or -not [bool]$witness.production_binary_commit_verified) { Fail "package checksum/commit verification incomplete" }
if (-not [bool]$witness.first_ready -or -not [bool]$witness.restart_ready -or -not [bool]$witness.restart_resume_detected) { Fail "READY/restart/Resume witness incomplete" }
if ([string]$witness.runtime_no_api_marker_scan -ne 'PASS' -or [string]$witness.w55_static_no_api_gate -ne 'PASS') { Fail "NO-API witness failed" }
if (-not [bool]$witness.no_upstream_fallback) { Fail "upstream fallback witness failed" }
if ([string]$witness.chapter_1_sha256_before_restart -ne [string]$witness.chapter_1_sha256_after_restart) { Fail "chapter 1 persistence mismatch" }
$completed = @($witness.final_progress.completed_chapters | ForEach-Object { [int]$_ })
if ($completed -notcontains 1 -or $completed -notcontains 2) { Fail "published binary did not complete chapters 1 and 2" }
$tools = @($witness.observed_local_tools | ForEach-Object { [string]$_ })
foreach ($tool in @('draft_chapter','commit_chapter')) { if ($tools -notcontains $tool) { Fail "missing local tool witness: $tool" } }

$evidence = [ordered]@{
    schema = 'ainovel-w6d-published-windows/1'
    repository = 'tiktok1997af-dot/ainovel-cli'
    tag = $ReleaseTag
    release_commit = $ExpectedGitSha
    binary_source = 'github-release-live-asset'
    archive_name = $archiveName
    archive_url = $expectedUrl
    checksum_url = $checksumUrl
    archive_sha256 = [string]$witness.package_archive_sha256
    production_binary_sha256 = [string]$witness.production_binary_sha256
    desktop_build_performed = $false
    package_checksum_verified = $true
    production_binary_commit_verified = $true
    first_ready = $true
    restart_ready = $true
    restart_resume_detected = $true
    chapter_1_sha256_before_restart = [string]$witness.chapter_1_sha256_before_restart
    chapter_1_sha256_after_restart = [string]$witness.chapter_1_sha256_after_restart
    completed_chapters = $completed
    observed_local_tools = $tools
    runtime_no_api_marker_scan = 'PASS'
    w55_static_no_api_gate = 'PASS'
    no_upstream_fallback = $true
}
$out = Join-Path $EvidenceDir 'w6d-published-windows-evidence.json'
[System.IO.File]::WriteAllText($out, (($evidence | ConvertTo-Json -Depth 8) + "`r`n"), (New-Object System.Text.UTF8Encoding($false)))
Write-Host 'W6D PUBLISHED WINDOWS WEB-ONLY DESKTOP AUTHORITY: PASS'
Write-Host "Release: $ReleaseTag / $ExpectedGitSha"
Write-Host "Archive: $archiveName"
