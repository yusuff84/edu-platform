param(
    [Parameter(Mandatory = $true)][string]$DatabaseUrl,
    [Parameter(Mandatory = $true)][string]$BackupPath,
    [Parameter(Mandatory = $true)][string]$UploadsPath
)
$ErrorActionPreference = 'Stop'
$previousDatabase = $env:PGDATABASE
try {
    $manifest = Get-Content -LiteralPath (Join-Path $BackupPath 'manifest.json') -Raw | ConvertFrom-Json
    if ($manifest.version -ne 1) { throw 'Unsupported backup format.' }
    $dump = Join-Path $BackupPath 'database.dump'
    $archive = Join-Path $BackupPath 'uploads.zip'
    if ((Get-FileHash -LiteralPath $dump -Algorithm SHA256).Hash -ne $manifest.databaseSha256) { throw 'Database backup checksum mismatch.' }
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $manifest.uploadsSha256) { throw 'Uploads backup checksum mismatch.' }
    if ((Test-Path -LiteralPath $UploadsPath) -and (Get-ChildItem -LiteralPath $UploadsPath -Force | Select-Object -First 1)) { throw 'Restore requires an empty uploads directory.' }
    $env:PGDATABASE = $DatabaseUrl
    $tableCount = & psql --dbname $DatabaseUrl --tuples-only --no-align --command "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')"
    if ($LASTEXITCODE -ne 0) { throw 'Cannot check destination database.' }
    if ([int]($tableCount.Trim()) -ne 0) { throw 'Restore requires an empty database; existing data will not be overwritten.' }
    & pg_restore --dbname $DatabaseUrl --exit-on-error --single-transaction --no-owner $dump
    if ($LASTEXITCODE -ne 0) { throw 'Database restore failed; transaction rolled back.' }
    if (-not (Test-Path -LiteralPath $UploadsPath)) { New-Item -ItemType Directory -Path $UploadsPath | Out-Null }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::ExtractToDirectory((Resolve-Path -LiteralPath $archive).Path, (Resolve-Path -LiteralPath $UploadsPath).Path)
    Write-Output 'Restore complete. Verify API readiness and uploaded files before routing users to this environment.'
} finally { $env:PGDATABASE = $previousDatabase }
