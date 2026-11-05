#requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = $env:WEAVE_IMAGE_VERSION,
    [string]$BinDir = $env:WEAVE_IMAGE_BIN_DIR,
    [switch]$NoPath,
    [switch]$NoAi
)

$ErrorActionPreference = "Stop"
$Repo = "prulloac/weave-image"
$Binary = "weave-image"

if (-not $Version) { $Version = "latest" }
if (-not $BinDir) {
    $BinDir = Join-Path $env:LOCALAPPDATA "Programs\weave\bin"
}
$SkipPath = $NoPath.IsPresent
if ($env:WEAVE_IMAGE_NO_MODIFY_PATH -eq "1") { $SkipPath = $true }

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($Version -eq "latest") {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "weave-image-installer" }
    $Tag = $release.tag_name
} else {
    $Tag = $Version
    if (-not $Tag.StartsWith("v")) { $Tag = "v$Tag" }
}
$VersionNumber = $Tag.TrimStart("v")

switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$Archive = "${Binary}_${VersionNumber}_windows_${Arch}.zip"
$Checksums = "${Binary}_${VersionNumber}_checksums.txt"
$Base = if ($env:WEAVE_IMAGE_RELEASE_BASE) { $env:WEAVE_IMAGE_RELEASE_BASE } else { "https://github.com/$Repo/releases/download/$Tag" }
$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("weave-image-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Temp -Force | Out-Null

try {
    $ArchivePath = Join-Path $Temp $Archive
    $ChecksumsPath = Join-Path $Temp $Checksums
    Invoke-WebRequest -Uri "$Base/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$Base/$Checksums" -OutFile $ChecksumsPath

    $Line = Get-Content $ChecksumsPath | Where-Object { ($_ -split '\s+')[1] -eq $Archive } | Select-Object -First 1
    if (-not $Line) { throw "Checksum not found for $Archive" }
    $Expected = ($Line -split '\s+')[0].ToLower()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum verification failed for $Archive" }

    $Extract = Join-Path $Temp "extract"
    Expand-Archive -Path $ArchivePath -DestinationPath $Extract -Force
    $Source = Join-Path $Extract "$Binary.exe"
    if (-not (Test-Path $Source)) { throw "Archive does not contain $Binary.exe" }

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $Target = Join-Path $BinDir "$Binary.exe"
    Copy-Item -Path $Source -Destination $Target -Force

    if (-not $SkipPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if (-not $UserPath) { $UserPath = "" }
        $Parts = @($UserPath -split ";" | Where-Object { $_ })
        if ($Parts -notcontains $BinDir) {
            $NewPath = if ($UserPath) { "$BinDir;$UserPath" } else { $BinDir }
            [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        }
        $env:Path = "$BinDir;$env:Path"
        Write-Host "Added $BinDir to the user PATH. Restart your terminal to use it everywhere."
    } else {
        Write-Host "Add $BinDir to your PATH manually if needed."
    }

    $skipAI = $NoAi.IsPresent -or ($env:WEAVE_IMAGE_NO_AI -eq "1")
    if (-not $skipAI) {
        $AITarget = Join-Path $BinDir "weave-ai.exe"
        $AICommand = $null
        if (Test-Path $AITarget) {
            $AICommand = $AITarget
        } else {
            $AICommand = (Get-Command "weave-ai.exe" -ErrorAction SilentlyContinue).Source
        }
        $AINeedsInstall = $true
        if ($AICommand) {
            $AIVersion = (& $AICommand --version 2>$null | Select-Object -First 1).Split(" ")[1]
            if ($AIVersion) {
                $Parsed = $AIVersion.TrimStart("v")
                if ($Parsed -match '^\d+\.\d+\.\d+$' -and [version]$Parsed -ge [version]"1.0.0") {
                    $AINeedsInstall = $false
                    Write-Host "weave-ai $AIVersion is already installed"
                }
            }
        }
        if ($AINeedsInstall) {
            Write-Host "Installing weave-ai >= 1.0.0"
            Invoke-Expression (Invoke-RestMethod "https://raw.githubusercontent.com/prulloac/weave-ai/main/scripts/install.ps1")
            $AICommand = Join-Path $BinDir "weave-ai.exe"
            if (-not (Test-Path $AICommand)) { throw "weave-ai installation failed" }
            $AIVersion = (& $AICommand --version 2>$null | Select-Object -First 1).Split(" ")[1]
            $Parsed = $AIVersion.TrimStart("v")
            if (-not ($Parsed -match '^\d+\.\d+\.\d+$') -or [version]$Parsed -lt [version]"1.0.0") {
                throw "weave-ai $AIVersion is older than 1.0.0"
            }
        }
    } else {
        Write-Host "Skipping weave-ai installation (-NoAi)"
    }

    Write-Host "Installed $Binary $Tag to $Target"
    & $Target --version
} finally {
    Remove-Item -Path $Temp -Recurse -Force -ErrorAction SilentlyContinue
}
