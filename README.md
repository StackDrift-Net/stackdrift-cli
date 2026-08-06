# StackDrift CLI

A command line tool for StackDrift. It scans a directory, detects the
technologies and dependency manifests that StackDrift supports, and adds them to
one of your StackDrift systems. StackDrift then tracks versions, end of life
dates, and security advisories for you.

## Install

### Linux/MacOS

```
curl -fsSL https://raw.githubusercontent.com/StackDrift-Net/stackdrift-cli/main/scripts/install.sh | bash
```

This installs the binary into a directory that is already on your PATH, such as
`~/.local/bin` or `/usr/local/bin`, so you can run `stackdrift` from anywhere
without changing any environment variables. Set `STACKDRIFT_INSTALL_DIR` to
force a specific directory.

The Linux script also works on macOS. It picks the right binary for Intel or
Apple Silicon automatically.

### Windows

Open PowerShell and run:

```
irm https://raw.githubusercontent.com/StackDrift-Net/stackdrift-cli/main/scripts/install.ps1 | iex
```

This installs the binary into `%LOCALAPPDATA%\Microsoft\WindowsApps`, which is
already on your PATH, so you can run `stackdrift` from anywhere without changing
any environment variables.

## Updating

To upgrade to the latest release:

```
stackdrift update
```

It downloads the newest binary for your platform and replaces the one you are
running. If you are already on the latest version it does nothing. Pass
`--force` to reinstall anyway. Install the CLI somewhere you can write to, such
as the default `~/.local/bin`, so the update can replace it in place without
extra permissions.

## Sign in

```
stackdrift login
```

This prints a link and a short code. Open the link in your browser, sign in to
StackDrift, and confirm the code matches. The CLI waits until you approve, then
saves a token so you do not need to sign in again. The token is stored in your
user config directory, not in your system.

To sign out:

```
stackdrift logout
```

## Track a directory

From inside the directory you want to track:

```
stackdrift scan
```

The first time, it asks whether to add the directory to an existing system or
create a new one. It then lists the technologies and dependency manifests it
found. Use the numbers to toggle items on or off, then press Enter. The CLI adds
the selected items to your system and records what is tracked.

That record is stored in `~/.stackdrift/<project-id>/.stackdrift`, not in the
directory you scanned. Scan targets are often public web roots, so nothing is
written where a web server could serve it. On later runs the CLI matches the
directory to its system and does not ask you to pick one again.

To accept everything without prompts, for example on a first automated run:

```
stackdrift scan --yes
```

This needs the system to be chosen once interactively in that directory first.

## Keeping it up to date automatically

After a scan the CLI offers to install a background service that notices when
what you already track has moved, and updates the system for you. The answer is
remembered per machine, including a no, so you are only asked once.

```
stackdrift service install          choose how often it checks
stackdrift service status           show whether it is installed and running
stackdrift service auto-update on   keep the CLI itself up to date
stackdrift service uninstall        remove it
stackdrift watch                    run one check now, in the foreground
```

Three intervals are offered: **daily**, **every other day** and **weekly**.
Daily is the recommendation. Advisories are published on a scale of days, so a
shorter interval finds the same news sooner than anyone can act on it while
asking more of the machine.

Pick the interval when installing to skip the question:

```
stackdrift service install --interval daily
```

`daily`, `everyotherday` and `weekly` are what the menu offers. `realtime`,
`5m`, `hourly` and `twicedaily` are still accepted here, and installs already
running on them keep working, but they are no longer put to anyone choosing.
`realtime` is the only one that stays resident; see what it costs, below.

### Keeping the CLI itself up to date

Installing the service also asks whether each scheduled check should install a
newer CLI release before it scans. The default is yes. It runs first, ahead of
everything else the check does, so a build the server no longer supports has
already replaced itself before it asks the server anything.

```
stackdrift service auto-update on
stackdrift service auto-update off
stackdrift service install --auto-update      answer without being asked
stackdrift service install --no-auto-update
```

`stackdrift service status` reports whether it is on and which release it last
installed.

Only the run the scheduler starts will ever replace the binary. Typing
`stackdrift watch` yourself never does, and neither does an install that predates
this feature: those stay as they are until you turn it on.

The release comes from the public GitHub releases for the CLI, pinned to the tag
the release feed named. Before anything is replaced the download has to be a real
executable for this platform and has to run and report the version it claims to
be. The binary it displaces is kept beside it as `stackdrift.prev`, and is put
back if the swap fails.

The check calls out at most once every 12 hours, so a machine on one of the
shorter intervals does not call GitHub on every sweep.

`--interval realtime` is the exception on Windows. A resident watcher can only
take a new binary if something restarts it, and a scheduled task will not, so
the resident Windows watcher never replaces itself and `service status` says so.
Linux and macOS restart theirs, so it replaces the binary and stands down for the
new one.

Auto-update needs to be able to write the directory the CLI is installed in. On
Linux the service runs in a sandbox that makes the whole filesystem read-only
apart from what it is given, so turning auto-update on is what adds that one
directory to the unit. If the directory is not writable at all, for example a
root-owned `/usr/local/bin` under a user service, it says so once and stops
checking rather than failing every run for ever.

### What it will and will not do

It keeps current what the system already tracks:

- a tracked dependency group is re-uploaded when its manifest or lock file
  contents change
- a tracked technology is replaced when its version has **demonstrably** moved,
  so advisories follow the version actually installed

Demonstrably is the important word. It acts only where exactly one row of a
technology is tracked and exactly one version of it is detected, and the two
disagree. That is the only shape in which "this install moved" is the sole
explanation. Every other shape is left alone:

| what it sees | what it does |
| --- | --- |
| two tracked rows of one technology | nothing. Another machine, or the website, may own the second row |
| two versions detected | nothing. A Dockerfile naming one release while the host runs another is not a move |
| nothing detected | nothing. An unmounted volume and an uninstall look identical |
| a scanned directory it could not read | nothing is retired that sweep, for any technology |
| a version the catalog could not resolve | nothing for that technology, since the two sides are not comparable |

The new version is always added **before** the old one is retired, so a crash or
a network drop in between leaves the system tracking both rather than neither.
The next sweep clears up the extra.

It deliberately does not:

- **add software you have not chosen.** A technology you were shown and left
  unticked is indistinguishable from one you have never seen, so neither is
  picked up. Run `stackdrift scan` again when you install something new.
- **remove software.** The only row it ever deletes is one that a move has
  already replaced. A technology that simply stops being detected is kept,
  because dropping it would silently end the CVE alerts you installed this for.

### What it costs

Measured on the built binary, scanning a real system:

| | memory | when |
| --- | --- | --- |
| Any offered interval | nothing resident | between checks |
| One check | ~14 MB peak, under 0.1s | at each interval |
| One check, 89 GB tree with 534 manifests | ~29 MB peak, 0.3s | worst case measured |
| `--interval realtime`, waiting | ~12 MB resident | continuously |
| `--interval realtime`, checking | ~15 MB peak | only when a watched file has moved |

Every offered interval is handed to the platform's own scheduler, so nothing of
ours is running between two checks.

Near realtime, which is only reachable by asking for it, does not walk your tree
every ten seconds. It stats the handful of files it already knows about, the
manifests it read plus `/etc/os-release` and `/proc/version`, and only does real
work when one of them has actually changed. A full walk still runs hourly to
catch anything the watch set cannot see.

The service runs at low priority and idle IO, so it yields to everything else.

### Where it is installed

| platform | mechanism | location |
| --- | --- | --- |
| Linux | systemd user service and timer | `~/.config/systemd/user/stackdrift-watch.*` |
| macOS | launchd agent | `~/Library/LaunchAgents/net.stackdrift.watch.plist` |
| Windows | scheduled task | `StackDrift Watch` |

The answers, and what the last update check found, are kept in
`~/.stackdrift/watch.json`.

It runs as you, not as root, because the credentials it needs are in your own
config directory. On Linux the installer also enables lingering so the service
keeps running on a server nobody is logged in to.

The systemd unit is confined: `ProtectSystem=strict`, `ProtectHome=read-only`,
`NoNewPrivileges`, and exactly one writable path, `~/.stackdrift`. Turning
auto-update on adds a second one, the directory the CLI is installed in, and
nothing else.

## Check for CVEs in CI

```
stackdrift check
```

This prints the CVE status of the system and exits with a non-zero code if any
tracked technology or dependency has a known CVE. Use it in a pipeline to fail a
build when a new advisory appears.

## Exit codes

Every command uses the same codes, so a pipeline can tell a security finding
apart from a billing one:

```
0  success
1  the command failed, which is also how check reports that it found a CVE
3  the account has no active plan, so the change was refused
```

Code 3 is never a security result. It covers an account that has never chosen a
plan as well as one whose plan has lapsed, and either way it stops changes only,
so `status`, `check`, and `remove` keep working and can never return it. Only a
command that writes, such as `scan`, is refused, and it is refused before it
scans anything. A plan can be chosen or reactivated on the billing page. To
treat that refusal as a warning rather than a failed build:

```
stackdrift scan --yes || [ $? -eq 3 ]
```

## What it detects

Technologies:

- .NET Full Framework and .NET Core SDK, from `.csproj` target frameworks
- The exact .NET Core SDK a repository pins, from `global.json`
- Every .NET Core SDK and runtime installed on the machine
- .NET Core SDK and .NET Core Runtime, from a Dockerfile base image
- Laravel, from `composer.json`
- WordPress, from `wp-includes/version.php`
- The host operating system, from `/etc/os-release`
- The running kernel build, recorded against that operating system
- Operating systems named in a Dockerfile `FROM` line

The kernel is not offered as a technology of its own. A distribution services
its own kernel, so its build belongs to the release you are on rather than to
upstream mainline, and it is tracked there. The catalog decides this, so what a
scan offers follows the website without needing a new CLI.

.NET installs its releases side by side and leaves the old ones in place, so a
scan lists every SDK and runtime line it finds under the install roots, with the
exact patch each one is on. Older patches of the same line are not listed
separately, because an application rolls forward onto the newest patch installed
and that is the one it runs. Installs are found through `DOTNET_ROOT`,
`/etc/dotnet/install_location`, whichever `dotnet` is on the path, and the
standard locations for each platform.

These are found on the machine rather than in the directory being scanned, so
they are listed unticked. Tick the ones the system you are tracking actually
runs.

Dependency manifests:

- npm: `package.json`
- NuGet: `.csproj`

Each project becomes its own dependency group. Lock and version files next to a
manifest are included automatically so versions are pinned: `package-lock.json`
for npm, and `packages.lock.json` plus `Directory.Packages.props` for NuGet. A
solution with four `.csproj` files produces four groups.

Folders like `node_modules`, `bin`, `obj`, and `.git` are skipped.

WordPress is found wherever its core sits, so a standard install, a subdirectory
install, and a Bedrock tree at `web/wp` all work without configuration. If a
directory holds more than one install, each is listed separately with its path,
which is how a forgotten copy at an old version shows up. Copies under
`wp-content/uploads` are ignored, because that is where backup plugins leave
snapshots of the whole site rather than anything you are running.

## Other commands

```
stackdrift status      show the tracked technologies and dependencies
stackdrift check       report CVE status and exit non-zero if any are found
stackdrift remove      remove technologies or dependencies from the system
stackdrift service     manage the background service that watches for changes
stackdrift watch       check now for stack changes and update StackDrift
stackdrift whoami      show the signed in account
stackdrift update      download and install the latest release
stackdrift version     print the CLI version
```

## Pointing at a different server

The CLI always talks to the public StackDrift server at https://stackdrift.net.
The only way to point it at a different server is the `STACKDRIFT_URL`
environment variable at runtime:

```
STACKDRIFT_URL=http://localhost:5000 stackdrift login
```

## Building from source

You need Go installed. To build release binaries for Linux, Windows, and macOS
(amd64 and arm64) into `dist/`:

```
scripts/build.sh 0.1.0
```

Every binary targets https://stackdrift.net. There is no build-time server
option; use the `STACKDRIFT_URL` environment variable to point at another
server at runtime.

To run the tests:

```
go test ./...
```
