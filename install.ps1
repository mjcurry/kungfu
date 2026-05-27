# kungfu installer for Windows.
#
# Usage (PowerShell 5.1+, PowerShell 7+ recommended):
#
#   irm https://raw.githubusercontent.com/mjcurry/kungfu/main/install.ps1 | iex
#
# Or, to inspect the script before running it:
#
#   iwr -useb https://raw.githubusercontent.com/mjcurry/kungfu/main/install.ps1 -OutFile install.ps1
#   .\install.ps1
#
# Environment variables (set before running):
#   $env:KUNGFU_VERSION           Version tag to install (default: latest)
#   $env:KUNGFU_INSTALL_DIR       Install destination (default:
#                                 $env:LOCALAPPDATA\Programs\kungfu)
#   $env:KUNGFU_INSTALL_DEBUG     Set to "1" for verbose output
#   $env:KUNGFU_INSTALL_BASE_URL  Override the archive base URL (CI use only)

$ErrorActionPreference = 'Stop'
# Force TLS 1.2 on Windows PowerShell 5.1, which defaults to TLS 1.0 and
# fails against modern GitHub endpoints.
[Net.ServicePointManager]::SecurityProtocol =
    [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'mjcurry/kungfu'

# -- logging ----------------------------------------------------------------

function Write-KfInfo {
    param([string]$Message)
    Write-Host $Message
}

function Write-KfErr {
    param([string]$Message)
    Write-Host "error: $Message" -ForegroundColor Red
}

function Write-KfDebug {
    param([string]$Message)
    if ($env:KUNGFU_INSTALL_DEBUG) {
        Write-Host "debug: $Message" -ForegroundColor DarkGray
    }
}

# -- platform detection -----------------------------------------------------

function Get-KfArch {
    # PROCESSOR_ARCHITEW6432 is set when a 32-bit process runs on a 64-bit
    # OS (WOW64); the underlying OS architecture is what we want for
    # picking a native release archive.
    $arch = if ($env:PROCESSOR_ARCHITEW6432) {
        $env:PROCESSOR_ARCHITEW6432
    } else {
        $env:PROCESSOR_ARCHITECTURE
    }
    switch ($arch) {
        'AMD64'  { return 'amd64' }
        'x86_64' { return 'amd64' }
        'ARM64'  { return 'arm64' }
        default {
            throw "unsupported architecture: $arch. See https://github.com/$Repo/releases for available binaries."
        }
    }
}

# -- version discovery ------------------------------------------------------

function Get-KfLatestTag {
    $api = "https://api.github.com/repos/$Repo/releases/latest"
    Write-KfDebug "discovering latest version from $api"
    $headers = @{
        'User-Agent' = 'kungfu-installer'
        'Accept'     = 'application/vnd.github+json'
    }
    $resp = Invoke-RestMethod -UseBasicParsing -Uri $api -Headers $headers
    if (-not $resp.tag_name) {
        throw "could not discover latest release from $api"
    }
    return $resp.tag_name
}

# -- checksum verification --------------------------------------------------

function Test-KfChecksum {
    param(
        [string]$ArchivePath,
        [string]$ChecksumPath
    )
    $name = [System.IO.Path]::GetFileName($ArchivePath)
    $actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()

    # goreleaser writes "<hash>  <filename>" lines (two-space separator,
    # coreutils style). Split on any whitespace and match column 2.
    $expected = $null
    foreach ($line in Get-Content -LiteralPath $ChecksumPath) {
        $fields = $line -split '\s+', 2
        if ($fields.Count -ge 2 -and $fields[1].Trim() -eq $name) {
            $expected = $fields[0].ToLowerInvariant()
            break
        }
    }
    if (-not $expected) {
        throw "checksum for $name not found in $ChecksumPath"
    }
    if ($actual -ne $expected) {
        throw "checksum mismatch for $name`n  expected: $expected`n  got:      $actual"
    }
    Write-KfDebug "checksum verified: $actual"
}

# -- install destination ----------------------------------------------------

function Get-KfInstallDir {
    if ($env:KUNGFU_INSTALL_DIR) {
        $dir = $env:KUNGFU_INSTALL_DIR
    } else {
        $dir = Join-Path $env:LOCALAPPDATA 'Programs\kungfu'
    }
    if (-not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    return (Resolve-Path -LiteralPath $dir).Path
}

# -- PATH update ------------------------------------------------------------

function Add-KfToUserPath {
    param([string]$Dir)
    $userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
    if (-not $userPath) { $userPath = '' }
    $normalized = [System.IO.Path]::GetFullPath($Dir)
    foreach ($p in ($userPath -split ';' | Where-Object { $_ -ne '' })) {
        try {
            if ([System.IO.Path]::GetFullPath($p) -eq $normalized) {
                Write-KfDebug "$Dir already on user PATH"
                return $false
            }
        } catch {
            # Skip malformed PATH entries rather than failing the install.
        }
    }
    $newUserPath = if ($userPath) { "$userPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable('PATH', $newUserPath, 'User')
    # Also extend the current session so the user can run kungfu without
    # opening a new terminal.
    $env:PATH = "$env:PATH;$Dir"
    return $true
}

# -- main -------------------------------------------------------------------

$arch = Get-KfArch
Write-KfDebug "arch: $arch"

$version = if ($env:KUNGFU_VERSION) { $env:KUNGFU_VERSION } else { Get-KfLatestTag }
$versionClean = $version -replace '^v', ''
Write-KfDebug "version: $version"

$archiveName  = "kungfu_${versionClean}_windows_${arch}.zip"
$checksumName = "kungfu_${versionClean}_checksums.txt"

$baseUrl = if ($env:KUNGFU_INSTALL_BASE_URL) {
    $env:KUNGFU_INSTALL_BASE_URL.TrimEnd('/')
} else {
    "https://github.com/$Repo/releases/download/$version"
}

$tmp = Join-Path $env:TEMP "kungfu-install-$([Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
Write-KfDebug "tempdir: $tmp"

try {
    $archivePath  = Join-Path $tmp $archiveName
    $checksumPath = Join-Path $tmp $checksumName

    Write-KfInfo "downloading $archiveName ..."
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$archiveName" -OutFile $archivePath

    Write-KfInfo "verifying checksum ..."
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$checksumName" -OutFile $checksumPath
    Test-KfChecksum -ArchivePath $archivePath -ChecksumPath $checksumPath

    Write-KfInfo "extracting ..."
    Expand-Archive -LiteralPath $archivePath -DestinationPath $tmp -Force

    $stagedExe = Join-Path $tmp 'kungfu.exe'
    if (-not (Test-Path -LiteralPath $stagedExe)) {
        throw "kungfu.exe not found in archive after extraction"
    }

    $installDir = Get-KfInstallDir
    $target = Join-Path $installDir 'kungfu.exe'
    Write-KfInfo "installing to $target ..."
    Copy-Item -LiteralPath $stagedExe -Destination $target -Force

    Write-KfInfo "smoke-testing ..."
    & $target version | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "smoke test failed: $target version exited with $LASTEXITCODE"
    }

    $pathAdded = Add-KfToUserPath -Dir $installDir

    Write-KfInfo ""
    Write-KfInfo "kungfu $version installed to $target"
    if ($pathAdded) {
        Write-KfInfo ""
        Write-KfInfo "added $installDir to your user PATH."
        Write-KfInfo "open a new terminal for the change to take effect."
    }
    Write-KfInfo ""
    Write-KfInfo "run 'kungfu --help' to get started."
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
