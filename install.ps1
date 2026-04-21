# agent-lsp installer for Windows
# Usage: iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex
#
# Options (set as env vars before running):
#   $env:AGENT_LSP_INSTALL_DIR = "C:\custom\path"   # override install directory
#
# macOS/Linux: use install.sh instead.

$ErrorActionPreference = 'Stop'

$Repo    = "blackwell-systems/agent-lsp"
$Binary  = "agent-lsp.exe"

# Detect architecture
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default  { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# Fetch latest release metadata
Write-Host "Fetching latest release..."
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
$Tag     = $Release.tag_name

if (-not $Tag) {
    throw "Could not determine latest release tag."
}

# Find the matching asset
$AssetName = "agent-lsp_windows_${Arch}.zip"
$Asset     = $Release.assets | Where-Object { $_.name -eq $AssetName } | Select-Object -First 1

if (-not $Asset) {
    throw "No release asset found for windows/$Arch.`nVisit: https://github.com/$Repo/releases/tag/$Tag"
}

Write-Host "Installing agent-lsp $Tag for windows/$Arch..."

# Determine install directory
# Custom override > admin (ProgramFiles) > user (LocalAppData)
if ($env:AGENT_LSP_INSTALL_DIR) {
    $InstallDir = $env:AGENT_LSP_INSTALL_DIR
} elseif ([bool](([System.Security.Principal.WindowsIdentity]::GetCurrent()).Groups -match "S-1-5-32-544")) {
    $InstallDir = Join-Path $env:ProgramFiles "agent-lsp"
} else {
    $InstallDir = Join-Path $env:LOCALAPPDATA "agent-lsp"
}

# Download to a temp directory
$TmpDir = Join-Path $env:TEMP ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    $ZipPath = Join-Path $TmpDir "agent-lsp.zip"
    Write-Host "Downloading from $($Asset.browser_download_url)..."
    Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $ZipPath -UseBasicParsing

    Write-Host "Extracting..."
    Expand-Archive -Path $ZipPath -DestinationPath $TmpDir -Force

    # Locate binary — GoReleaser may nest it in a subdirectory
    $BinaryPath = Get-ChildItem -Path $TmpDir -Recurse -Filter $Binary | Select-Object -First 1
    if (-not $BinaryPath) {
        throw "Could not find $Binary in downloaded archive"
    }

    # Install
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $BinaryPath.FullName -Destination (Join-Path $InstallDir $Binary) -Force
} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

# Add to user PATH if not already present
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*${InstallDir}*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    $env:PATH = "$env:PATH;$InstallDir"
    Write-Host "Added $InstallDir to PATH"
} else {
    Write-Host "$InstallDir already in PATH"
}

# Verify
try {
    $Version = & (Join-Path $InstallDir $Binary) --version 2>$null
} catch {
    $Version = $Tag
}

Write-Host ""
Write-Host "Installed agent-lsp $Version"
Write-Host "  Binary: $InstallDir\$Binary"
Write-Host ""
Write-Host "Restart your terminal, then run 'agent-lsp init' to configure your AI tool."
