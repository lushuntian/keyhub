param(
  [string]$BackupDir = (Join-Path (Split-Path -Parent $PSScriptRoot) "backups")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backupFile = Join-Path $BackupDir "keyhub-$timestamp.sql"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)

Push-Location $projectRoot
try {
  $dump = & docker compose exec -T mysql sh -c 'mysqldump --default-character-set=utf8mb4 -ukeyhub -p"$MYSQL_PASSWORD" --single-transaction --routines --triggers keyhub'
  if ($LASTEXITCODE -ne 0) {
    throw "mysqldump failed with exit code $LASTEXITCODE"
  }
  [System.IO.File]::WriteAllText($backupFile, ($dump -join [Environment]::NewLine), $utf8NoBom)
  Write-Host "Backup written to $backupFile"
} finally {
  Pop-Location
}
