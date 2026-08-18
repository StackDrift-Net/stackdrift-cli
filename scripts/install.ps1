# Installs the StackDrift CLI on Windows into Program Files, excludes that one
# directory from Microsoft Defender, and puts it on the machine PATH. Run from an
# Administrator PowerShell with:
#   irm https://raw.githubusercontent.com/StackDrift-Net/stackdrift-cli/main/scripts/install.ps1 | iex
#
# Set $env:STACKDRIFT_PER_USER = "1" first to install into LOCALAPPDATA instead.
# That needs no elevation and adds no exclusion, so Defender is free to remove the
# binary and the scheduled task with it, which is what it has been doing.

$ErrorActionPreference = "Stop"

$repo = "StackDrift-Net/stackdrift-cli"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$binary = "stackdrift-windows-$arch.exe"
$url = "https://github.com/$repo/releases/latest/download/$binary"
$taskName = "StackDrift Watch"
$perUserDir = Join-Path $env:LOCALAPPDATA "StackDrift"

$perUser = -not [string]::IsNullOrWhiteSpace($env:STACKDRIFT_PER_USER)
$elevated = (New-Object Security.Principal.WindowsPrincipal(
    [Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

# Defender treats the CLI as a threat: it is an unsigned Go binary with no
# reputation that registers a scheduled task and replaces its own executable, and
# the verdict comes with persistence remediation, so it deletes the task and its
# registry entries along with the file.
#
# The remedy is an exclusion, and the reason the install directory moved is that
# it decides whether that exclusion is safe. Excluding LOCALAPPDATA\StackDrift
# hands anything running as the signed-in user a folder it can write to and have
# skipped, which is a genuine hole. Program Files can only be written by an
# administrator, and an administrator can turn the exclusion off anyway, so
# excluding it grants nothing that was not already held.
#
# It is still a change to somebody's antivirus configuration, so it is announced
# rather than done quietly, and a refusal is reported rather than worked around.
function Add-DefenderExclusion($dir) {
    if (-not (Get-Command Add-MpPreference -ErrorAction SilentlyContinue)) {
        Write-Host "Microsoft Defender is not present, so no exclusion was added."
        Write-Host "If another antivirus product manages this machine, exclude $dir there."
        return
    }

    $existing = @()
    try { $existing = (Get-MpPreference).ExclusionPath } catch { $existing = @() }
    foreach ($path in $existing) {
        if ($path -and ($path.TrimEnd('\') -ieq $dir.TrimEnd('\'))) {
            Write-Host "Defender already excludes $dir."
            return
        }
    }

    try {
        Add-MpPreference -ExclusionPath $dir -ErrorAction Stop
        Write-Host "Excluded $dir from Microsoft Defender scanning."
    }
    catch {
        # Tamper Protection and managed-endpoint policy both refuse this, and
        # neither is ours to override.
        Write-Host "Could not add the Defender exclusion: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "Ask whoever manages this machine to exclude $dir, or the CLI will keep being removed."
    }
}

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

if (-not $perUser -and -not $elevated) {
    Write-Host "This installs into Program Files and excludes that directory from Defender." -ForegroundColor Yellow
    Write-Host "Both need administrator rights. Open an Administrator PowerShell and run:"
    Write-Host ""
    Write-Host "    irm https://raw.githubusercontent.com/$repo/main/scripts/install.ps1 | iex"
    Write-Host ""
    Write-Host "To install into your own profile instead, without the exclusion, run:"
    Write-Host ""
    Write-Host "    `$env:STACKDRIFT_PER_USER = `"1`"; irm https://raw.githubusercontent.com/$repo/main/scripts/install.ps1 | iex"
    Write-Host ""
    Write-Host "Defender removes the per-user copy and its scheduled task, so the background checks stop."
    # return, never exit: this script is piped into iex, and exit would close
    # the user's shell.
    return
}

Write-Host "Installing the StackDrift CLI"

# One install directory, and by default one only an administrator can write.
#
# WindowsApps was tried first because it is already on PATH, but it is the OS App
# Execution Alias folder: Windows servicing cleans it, alias entries can shadow a
# real binary of the same name, and being "on PATH" there is itself an alias
# mechanism rather than an ordinary directory entry. LOCALAPPDATA\StackDrift
# replaced it and had neither problem, but it is user-writable, which is what
# makes an exclusion there unsafe.
if ($perUser) {
    $installDir = $perUserDir
    $pathScope = "User"
}
else {
    # ProgramW6432 first. A 32-bit PowerShell on 64-bit Windows reads
    # ProgramFiles as the (x86) directory, and installing the 64-bit binary there
    # would work but would sit somewhere nobody would think to look.
    $programFiles = if ($env:ProgramW6432) { $env:ProgramW6432 } else { $env:ProgramFiles }
    $installDir = Join-Path $programFiles "StackDrift"
    $pathScope = "Machine"
}
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$target = Join-Path $installDir "stackdrift.exe"

# Before the download, not after it. Defender takes the file during the write, so
# an exclusion added afterwards would be excluding an empty directory.
if (-not $perUser) {
    Add-DefenderExclusion $installDir
}

Write-Host "Downloading $url"
Invoke-WebRequest -Uri $url -OutFile $target

# A quarantine happens after the download reports success, so the file is
# confirmed rather than assumed.
if (-not (Test-Path $target)) {
    Show-DefenderRemediation $installDir
    return
}

Write-Host "Installed to $target"

# Older copies, either of which would keep answering the command and make the
# install look like it had done nothing. WindowsApps sits ahead of both install
# directories on PATH; the per-user copy is behind a machine PATH entry but is
# still what a stale scheduled task points at.
$stale = @(Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\stackdrift.exe")
if (-not $perUser) {
    $stale += (Join-Path $perUserDir "stackdrift.exe")
}
foreach ($old in $stale) {
    if ((Test-Path $old) -and ($old -ne $target)) {
        Remove-Item $old -Force -ErrorAction SilentlyContinue
        if (Test-Path $old) {
            Write-Host "Could not remove the older copy at $old. Close anything running it and delete it by hand." -ForegroundColor Yellow
        }
        else {
            Write-Host "Removed an older copy from $(Split-Path -Parent $old)."
        }
    }
}

# The PERSISTED PATH, read from the registry, not $env:Path. The session variable
# is the machine PATH merged with whichever profile launched this shell, so in an
# elevated window it says nothing about what an ordinary terminal will see.
# Checking the wrong one is why an install could report success and then not be
# found in the next window.
$persisted = [Environment]::GetEnvironmentVariable("Path", $pathScope)
$onPath = $false
foreach ($part in (($persisted -split ';') | Where-Object { $_ })) {
    if ($part.TrimEnd('\') -ieq $installDir.TrimEnd('\')) { $onPath = $true }
}

if (-not $onPath) {
    $updated = if ([string]::IsNullOrWhiteSpace($persisted)) { $installDir } else { "$persisted;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $updated, $pathScope)
    Write-Host "Added $installDir to the $($pathScope.ToLower()) PATH."
}

# A machine install leaves the old per-user entry pointing at a directory this
# script has just emptied.
if (-not $perUser) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $kept = @(($userPath -split ';') | Where-Object { $_ -and ($_.TrimEnd('\') -ine $perUserDir.TrimEnd('\')) })
    if ($userPath -and ($kept.Count -ne (($userPath -split ';') | Where-Object { $_ }).Count)) {
        [Environment]::SetEnvironmentVariable("Path", ($kept -join ';'), "User")
        Write-Host "Removed the old per-user PATH entry for $perUserDir."
    }
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
    # beside the profile that loads it. It stays under LOCALAPPDATA even for a
    # machine install, because the profile is per user and Program Files is not
    # writable by the account that would be reading it. It asks the binary what
    # to offer, so it survives an update without being rewritten.
    New-Item -ItemType Directory -Force -Path $perUserDir | Out-Null
    $completionFile = Join-Path $perUserDir "completion.ps1"
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

# A task registered against the old location keeps running the binary this script
# has just deleted. An interactive scan repairs that on its own, but saying so
# beats waiting for someone to notice the checks had stopped.
try {
    # 2>$null because PowerShell 7 turns a native command's stderr into a
    # terminating error while ErrorActionPreference is Stop, and querying a task
    # that does not exist writes to stderr as its normal answer.
    & schtasks.exe /query /tn $taskName 2>$null | Out-Null
    $scheduled = ($LASTEXITCODE -eq 0)
}
catch { $scheduled = $false }

if ($scheduled) {
    Write-Host ""
    Write-Host "A background check is already scheduled against the previous location."
    Write-Host "Run 'stackdrift service install' to point it at $target."
}

Write-Host ""
Write-Host "Next: run 'stackdrift login' then 'stackdrift scan' in a project directory."
