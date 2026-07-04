param(
  [Parameter(Mandatory = $true)]
  [string]$BackupFile,

  [switch]$ConfirmRestore
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $ConfirmRestore) {
  throw "Pass -ConfirmRestore to restore into the keyhub database."
}

$resolvedBackup = (Resolve-Path -LiteralPath $BackupFile).Path
$content = [System.IO.File]::ReadAllText($resolvedBackup, [System.Text.Encoding]::UTF8)
if ([string]::IsNullOrWhiteSpace($content)) {
  throw "Backup file is empty: $resolvedBackup"
}
if ($content[0] -eq [char]0xFEFF) {
  $content = $content.Substring(1)
}

$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
  $content | & docker compose exec -T mysql sh -c 'mysql --default-character-set=utf8mb4 -ukeyhub -p"$MYSQL_PASSWORD" keyhub'
  if ($LASTEXITCODE -ne 0) {
    throw "mysql restore failed with exit code $LASTEXITCODE"
  }
  Write-Host "Restore completed from $resolvedBackup"
} finally {
  Pop-Location
}
