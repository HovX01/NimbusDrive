# Sync local Nimbus tree to the home server and rebuild the container.
# Uses SSH Host alias only (default: yuthomeserver). Does not change
# LAN IPs, Tailscale IPs, AdGuard DNS, Caddy, or SSH config.

param(
  [string]$SshHost = $(if ($env:NIMBUS_SSH_HOST) { $env:NIMBUS_SSH_HOST } else { "yuthomeserver" }),
  [string]$RemoteDir = $(if ($env:NIMBUS_REMOTE_DIR) { $env:NIMBUS_REMOTE_DIR } else { "~/apps/nimbus" }),
  [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path (Join-Path $Root "Dockerfile"))) {
  throw "Dockerfile not found at $Root"
}

Write-Host "SSH host: $SshHost"
Write-Host "Remote:   $RemoteDir"
Write-Host "Source:   $Root"

ssh -o BatchMode=yes -o ConnectTimeout=15 $SshHost "echo connected && hostname" | Out-Host

$stamp = Get-Date -Format "yyyyMMddHHmmss"
$tarName = "nimbus-sync-$stamp.tar"
$localTar = Join-Path $env:TEMP $tarName
$remoteTar = "/tmp/$tarName"

if (Test-Path $localTar) { Remove-Item $localTar -Force }

Push-Location $Root
try {
  # ustar for Linux tar; keep deploy/.env on the server (never overwrite)
  & tar --format=ustar -cf $localTar `
    --exclude=.git `
    --exclude=data `
    --exclude=node_modules `
    --exclude=web/node_modules `
    --exclude=testAPI/node_modules `
    --exclude=web/dist `
    --exclude=.env `
    --exclude=deploy/.env `
    --exclude=deploy/.env.server `
    --exclude=testAPI/config.js `
    --exclude=*.db `
    --exclude=*.db-* `
    --exclude=*.dump `
    --exclude=*.tar `
    --exclude=*.zip `
    --exclude=tmpcheck `
    --exclude=fetchall `
    --exclude=bin `
    .
} finally {
  Pop-Location
}

Write-Host "Uploading $($(Get-Item $localTar).Length) bytes..."
scp $localTar "${SshHost}:$remoteTar"

$remoteScript = @"
set -euo pipefail
REMOTE=$RemoteDir
TAR=$remoteTar
mkdir -p "`$REMOTE"
# preserve server secrets / runtime env
if [ -f "`$REMOTE/deploy/.env" ]; then
  cp "`$REMOTE/deploy/.env" /tmp/nimbus-deploy.env.bak
fi
tar -xf "`$TAR" -C "`$REMOTE"
rm -f "`$TAR"
mkdir -p "`$REMOTE/deploy"
if [ -f /tmp/nimbus-deploy.env.bak ]; then
  mv /tmp/nimbus-deploy.env.bak "`$REMOTE/deploy/.env"
elif [ ! -f "`$REMOTE/deploy/.env" ] && [ -f "`$REMOTE/deploy/.env.example" ]; then
  cp "`$REMOTE/deploy/.env.example" "`$REMOTE/deploy/.env"
  echo "created deploy/.env from example — fill Telegram credentials if needed"
fi
echo "files synced to `$REMOTE"
"@

ssh $SshHost $remoteScript

Remove-Item $localTar -Force -ErrorAction SilentlyContinue

if ($NoBuild) {
  Write-Host "Sync done (skipped rebuild)."
  exit 0
}

Write-Host "Building and restarting container..."
ssh $SshHost "cd $RemoteDir/deploy && docker compose -f compose.yml pull cobalt && docker compose -f compose.yml up -d --build && docker compose -f compose.yml ps && docker exec nimbus wget -qO- http://127.0.0.1:8080/health && echo && docker exec nimbus yt-dlp --version"

Write-Host "Done. App: https://nimbus.home.arpa"
Write-Host "Note: LAN/Tailscale/SSH IPs were not modified."
