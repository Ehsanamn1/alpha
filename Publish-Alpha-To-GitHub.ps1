$ErrorActionPreference = 'Stop'

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Required command '$Name' was not found. Install Git and GitHub CLI (gh) first."
  }
}
function Invoke-Git([string[]]$GitArgs) {
  & git @GitArgs
  if ($LASTEXITCODE -ne 0) { throw "git $($GitArgs -join ' ') failed with exit code $LASTEXITCODE" }
}
function Invoke-Gh([string[]]$GhArgs) {
  & gh @GhArgs
  if ($LASTEXITCODE -ne 0) { throw "gh $($GhArgs -join ' ') failed with exit code $LASTEXITCODE" }
}

Require-Command 'git'
Require-Command 'gh'

try { Invoke-Gh @('auth', 'status') } catch {
  Write-Host 'Sign in first by running: gh auth login' -ForegroundColor Yellow
  throw
}
$currentLogin = (& gh api user --jq .login).Trim()
if ($LASTEXITCODE -ne 0 -or $currentLogin -ne 'Ehsanamn1') {
  throw "Safety stop: GitHub CLI is authenticated as '$currentLogin'; expected Ehsanamn1."
}
Invoke-Gh @('auth', 'setup-git')

if (-not (Test-Path '.git')) {
  Invoke-Git @('init', '-b', 'main')
}
$branch = (& git branch --show-current).Trim()
if ($branch -ne 'main') {
  throw "Safety stop: current branch is '$branch'; switch to main before publishing."
}
if (-not (& git config user.name)) {
  $ghName = (& gh api user --jq '(.name // .login)').Trim()
  Invoke-Git @('config', 'user.name', $ghName)
}
if (-not (& git config user.email)) {
  $ghId = (& gh api user --jq '.id').Trim()
  Invoke-Git @('config', 'user.email', "$ghId+Ehsanamn1@users.noreply.github.com")
}

$expectedRepo = 'Ehsanamn1/alpha'
$origin = (& git remote get-url origin 2>$null)
if ($LASTEXITCODE -eq 0 -and $origin) {
  if ($origin -notmatch 'github\.com[:/]Ehsanamn1/alpha(?:\.git)?$') {
    throw "Safety stop: origin '$origin' is not $expectedRepo."
  }
} else {
  Invoke-Git @('remote', 'add', 'origin', 'https://github.com/Ehsanamn1/alpha.git')
}

Invoke-Git @('add', '-A')
& git diff --cached --quiet
if ($LASTEXITCODE -eq 1) {
  Invoke-Git @('commit', '-m', 'Publish Alpha 0.3.0-alpha source')
} elseif ($LASTEXITCODE -ne 0) {
  throw 'Could not inspect the staged Git diff.'
}

$remoteLine = (& git ls-remote origin refs/heads/main)
if ($LASTEXITCODE -ne 0) { throw 'Could not inspect the remote main branch.' }
$remoteSha = ''
if ($remoteLine) { $remoteSha = ($remoteLine -split '\s+')[0] }
if ($remoteSha) {
  Invoke-Git @('fetch', 'origin', 'main')
  $remoteFiles = @(& git ls-tree -r --name-only origin/main)
  if ($LASTEXITCODE -ne 0) { throw 'Could not inspect files on remote main.' }
  if ($remoteFiles.Count -gt 0) {
    throw "Safety stop: remote main contains files already. No force-push was made. Review https://github.com/$expectedRepo before continuing."
  }
  Invoke-Git @('push', "--force-with-lease=refs/heads/main:$remoteSha", '-u', 'origin', 'main')
} else {
  Invoke-Git @('push', '-u', 'origin', 'main')
}

$releaseTag = 'v0.3.0-alpha'
$existingTag = (& git ls-remote --tags origin "refs/tags/$releaseTag" "refs/tags/$releaseTag^{}")
if ($LASTEXITCODE -ne 0) { throw 'Could not inspect remote release tags.' }
if ($existingTag) {
  throw "The tag $releaseTag already exists remotely. Source was pushed, but the script will not move an existing release tag. Review GitHub Releases first."
}
Invoke-Git @('tag', $releaseTag)
Invoke-Git @('push', 'origin', $releaseTag)

Write-Host ''
Write-Host 'Alpha source pushed to https://github.com/Ehsanamn1/alpha' -ForegroundColor Green
Write-Host 'The tag v0.3.0-alpha was pushed. Check Actions for CI, multiplatform builds and release publication:'
Write-Host 'https://github.com/Ehsanamn1/alpha/actions'
Write-Host 'The release is not considered published until the GitHub Actions run completes successfully:'
Write-Host 'https://github.com/Ehsanamn1/alpha/releases'
