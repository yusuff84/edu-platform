param(
    [Parameter(Mandatory = $true)][string]$DatabaseUrl,
    [Parameter(Mandatory = $true)][string]$Destination,
    [Parameter(Mandatory = $true)][string]$UploadsPath
)
$ErrorActionPreference = 'Stop'
$previousDatabase = $env:PGDATABASE
try {
    if (Test-Path -LiteralPath $Destination) { throw 'Backup destination must not exist.' }
    if (-not (Test-Path -LiteralPath $UploadsPath -PathType Container)) { throw 'Uploads directory does not exist.' }
    $backupRoot = (New-Item -ItemType Directory -Path $Destination).FullName
    $env:PGDATABASE = $DatabaseUrl
    & pg_dump --dbname $DatabaseUrl --format=custom --file (Join-Path $backupRoot 'database.dump')
    if ($LASTEXITCODE -ne 0) { throw 'pg_dump failed; this backup is incomplete.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::CreateFromDirectory((Resolve-Path -LiteralPath $UploadsPath).Path, (Join-Path $backupRoot 'uploads.zip'))
    $manifest = @{
        version = 1
        createdAt = [DateTime]::UtcNow.ToString('o')
        databaseSha256 = (Get-FileHash -LiteralPath (Join-Path $backupRoot 'database.dump') -Algorithm SHA256).Hash
        uploadsSha256 = (Get-FileHash -LiteralPath (Join-Path $backupRoot 'uploads.zip') -Algorithm SHA256).Hash
    }
    $manifest | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $backupRoot 'manifest.json') -Encoding UTF8
    Write-Output "Backup complete: $backupRoot"
} finally { $env:PGDATABASE = $previousDatabase }
