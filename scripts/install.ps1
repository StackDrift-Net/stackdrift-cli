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

# Defender treats the CLI as a threat: it is an unsigned Go binary with no
# reputation, and the verdict comes with persistence remediation, so it deletes
# the scheduled task and its registry entries along with the executable.
#
# The remedy is an exclusion, and this script deliberately does not add one.
# An installer that quietly exempts itself from antivirus is indistinguishable
# from malware doing the same thing, it trips endpoint monitoring on managed
# machines, and the excluded directory is exactly where the CLI's own updates
# land. Whoever owns the machine decides, not us.
function Show-DefenderRemediation($dir) {
    Write-Host ""
    Write-Host "Windows Defender has removed the StackDrift binary." -ForegroundColor Yellow
    Write-Host "It is unsigned and newly published, which Defender treats as a threat. The"
    Write-Host "scheduled task that runs the background checks is deleted along with it."
    Write-Host ""
    Write-Host "To proceed, run this as Administrator and then run the installer again:"
    Write-Host ""
    Write-Host "    Add-MpPreference -ExclusionPath `"$dir`""
    Write-Host ""
    Write-Host "That stops the directory being scanned, including future updates downloaded"
    Write-Host "into it. This installer will not change your antivirus settings for you."
}

# One install directory, always. WindowsApps was tried first because it is
# already on PATH, but it is the OS App Execution Alias folder: Windows servicing
# cleans it, alias entries can shadow a real binary of the same name, and being
# "on PATH" there is itself an alias mechanism rather than an ordinary directory
# entry. A folder we own has none of those problems.
$installDir = Join-Path $env:LOCALAPPDATA "StackDrift"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$target = Join-Path $installDir "stackdrift.exe"

Write-Host "Downloading $url"
Invoke-WebRequest -Uri $url -OutFile $target

# Defender takes an interest in this binary, and a quarantine happens AFTER the
# download reports success, so the file is confirmed rather than assumed.
if (-not (Test-Path $target)) {
    Show-DefenderRemediation $installDir
    return
}

Write-Host "Installed to $target"

# An earlier version of this installer put the binary in WindowsApps, which is
# on PATH ahead of this directory. Left behind it would keep answering the
# command and the update would look like it had done nothing.
$stale = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\stackdrift.exe"
if ((Test-Path $stale) -and ($stale -ne $target)) {
    Remove-Item $stale -Force -ErrorAction SilentlyContinue
    if (-not (Test-Path $stale)) {
        Write-Host "Removed an older copy from WindowsApps that would have shadowed this one."
    }
}

# The PERSISTED user PATH, read from the registry, not $env:Path. The session
# variable is the machine PATH merged with whichever profile launched this
# shell, so in an elevated window it says nothing about what an ordinary
# terminal will see. Checking the wrong one is why an install could report
# success and then not be found in the next window.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$onUserPath = $false
foreach ($part in (($userPath -split ';') | Where-Object { $_ })) {
    if ($part.TrimEnd('\') -ieq $installDir.TrimEnd('\')) { $onUserPath = $true }
}

if (-not $onUserPath) {
    $updated = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { "$userPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $updated, "User")
    Write-Host "Added $installDir to your PATH."
}

# The current session too, so the command works here as well as in the next
# terminal. SetEnvironmentVariable only writes the registry.
if ($env:Path -notlike "*$installDir*") {
    $env:Path = "$env:Path;$installDir"
}

# Proves the binary can actually start. Defender blocks the first execution as
# readily as it deletes the download, and that is how this failed before: the
# install reported success, then died at the first use of the binary with an
# error about a virus, leaving the CLI half set up.
try {
    & $target version | Out-Null
}
catch {
    Show-DefenderRemediation $installDir
    return
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
