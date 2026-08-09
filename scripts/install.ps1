# Installs the StackDrift CLI on Windows. Downloads the release binary and
# places it in a directory that is already on your PATH, so no environment
# variable changes are needed. Run with:
#   irm https://raw.githubusercontent.com/StackDrift-Net/stackdrift-cli/main/scripts/install.ps1 | iex

$ErrorActionPreference = "Stop"

# Elevation is not something installing a per-user CLI should need, and it is
# only checked because of Windows Defender. The binary is unsigned and freshly
# published, so it has no reputation, and in a normal session Defender refuses
# to let it run at all. The download succeeds, then the first invocation of it
# fails with 0x800700E1 and the install stops half finished. Running elevated
# avoids that. Checked before anything is downloaded so nothing is left behind.
$elevated = (New-Object Security.Principal.WindowsPrincipal(
    [Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $elevated) {
    Write-Host "Run this from an Administrator PowerShell." -ForegroundColor Yellow
    Write-Host "Windows Defender blocks the newly downloaded binary from running otherwise, which leaves the install half finished."
    # return, never exit: this script is piped into iex, and exit would close
    # the user's shell.
    return
}

$repo = "StackDrift-Net/stackdrift-cli"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$binary = "stackdrift-windows-$arch.exe"
$url = "https://github.com/$repo/releases/latest/download/$binary"

Write-Host "Installing the StackDrift CLI"

function Test-OnPath($dir) {
    $target = $dir.TrimEnd('\')
    foreach ($part in ($env:Path -split ';')) {
        if ($part.TrimEnd('\') -ieq $target) { return $true }
    }
    return $false
}

$windowsApps = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps"

if ((Test-Path $windowsApps) -and (Test-OnPath $windowsApps)) {
    $target = Join-Path $windowsApps "stackdrift.exe"
    Write-Host "Downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $target
    Write-Host "Installed to $target"
    Write-Host "That directory is already on your PATH."
}
else {
    $installDir = Join-Path $env:LOCALAPPDATA "StackDrift"
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $target = Join-Path $installDir "stackdrift.exe"
    Write-Host "Downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $target
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
        Write-Host "Added $installDir to your PATH. Open a new terminal for it to take effect."
    }
    Write-Host "Installed to $target"
}

if (-not $env:STACKDRIFT_NO_COMPLETION) {
    # PowerShell has no drop-in completion directory, so the script is written
    # beside the binary and loaded from the profile. It asks the binary what to
    # offer, so it survives an update without being rewritten.
    $completionDir = Join-Path $env:LOCALAPPDATA "StackDrift"
    New-Item -ItemType Directory -Force -Path $completionDir | Out-Null
    $completionFile = Join-Path $completionDir "completion.ps1"
    & $target completion powershell | Set-Content -Path $completionFile -Encoding UTF8

    $alreadyLoaded = (Test-Path $PROFILE) -and ((Get-Content $PROFILE -Raw) -match [regex]::Escape($completionFile))
    if (-not $alreadyLoaded) {
        $profileDir = Split-Path -Parent $PROFILE
        if (-not (Test-Path $profileDir)) { New-Item -ItemType Directory -Force -Path $profileDir | Out-Null }
        Add-Content -Path $PROFILE -Value "`n# StackDrift tab completion`n. `"$completionFile`""
        Write-Host ""
        Write-Host "Added tab completion to your PowerShell profile ($PROFILE)."
    }
    Write-Host "Tab completion installed. Open a new terminal to use it."
}

Write-Host ""
Write-Host "Next: run 'stackdrift login' then 'stackdrift scan' in a project directory."
