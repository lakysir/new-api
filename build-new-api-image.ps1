param(
    [string]$ImageName = "new-api-custom:latest",
    [string]$TarName = "new-api-custom.tar",
    [switch]$SkipGitPull
)

$ErrorActionPreference = "Stop"

# This script lives in the new-api/ project root, so its own directory is the
# build context. Run it from anywhere: paths are resolved relative to the file.
$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

Set-Location $RepoRoot

if (-not $SkipGitPull) {
    Write-Host "[1/4] Pull latest code"
    git pull --ff-only
} else {
    Write-Host "[1/4] Skip git pull"
}

# Build on the docker driver (the desktop-linux/default builder), NOT the buildx
# docker-container driver. The container driver keeps its own cache and ignores
# daemon.json registry-mirrors, so it hits docker.io directly and times out on a
# throttled network. The docker driver shares the daemon image store + mirrors,
# so already-pulled base images resolve offline.
Write-Host "[2/4] Build Docker image: $ImageName"
docker buildx build --builder desktop-linux --load -f Dockerfile.local -t $ImageName .

Write-Host "[3/4] Save Docker image to: $TarName"
if (Test-Path $TarName) {
    Remove-Item $TarName -Force
}
docker save $ImageName -o $TarName

Write-Host "[4/4] Done"
Write-Host "Image tar: $(Join-Path $RepoRoot $TarName)"
Write-Host "R2 credentials are runtime secrets and are not embedded in the image."
Write-Host "When starting the container, pass the server-side .env with --env-file or configure the R2_* environment variables in Docker Compose / your container panel."
