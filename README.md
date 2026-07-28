# StackDrift CLI

A command line tool for StackDrift. It scans a project directory, detects the
technologies and dependency manifests that StackDrift supports, and adds them to
one of your StackDrift projects. StackDrift then tracks versions, end of life
dates, and security advisories for you.

## Install

### Linux/MacOS

```
curl -fsSL https://raw.githubusercontent.com/digitalaffinity-au/stackdrift-cli/main/scripts/install.sh | bash
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
irm https://raw.githubusercontent.com/digitalaffinity-au/stackdrift-cli/main/scripts/install.ps1 | iex
```

This installs the binary into `%LOCALAPPDATA%\Microsoft\WindowsApps`, which is
already on your PATH, so you can run `stackdrift` from anywhere without changing
any environment variables.

## Tab completion

The installer sets up tab completion, so pressing Tab after `stackdrift`
completes the command names and the options each one takes:

```
stackdrift sc<Tab>        completes to "stackdrift scan"
stackdrift scan --<Tab>   offers "--yes"
```

Open a new terminal after installing for it to take effect. bash and fish need
nothing further. zsh loads it automatically when it can be written to
`/usr/local/share/zsh/site-functions`; otherwise the installer prints the one
line to add to your `~/.zshrc`. On Windows the installer adds a line to your
PowerShell profile, because PowerShell has no directory it loads completions
from on its own.

The completion asks the CLI itself what to offer, so it stays correct across
updates and never needs reinstalling. Set `STACKDRIFT_NO_COMPLETION=1` before
running the installer to skip it. To write a script yourself:

```
stackdrift completion bash > ~/.local/share/bash-completion/completions/stackdrift
```

`bash`, `zsh`, `fish`, and `powershell` are supported.

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
user config directory, not in your project.

To sign out:

```
stackdrift logout
```

## Track a directory

From inside a project directory:

```
stackdrift scan
```

The first time, it asks whether to add the directory to an existing project or
create a new one. It then lists the technologies and dependency manifests it
found. Use the numbers to toggle items on or off, then press Enter. The CLI adds
the selected items to your project and records what is tracked.

That record is stored in `~/.stackdrift/<project-id>/.stackdrift`, not in the
directory you scanned. Scan targets are often public web roots, so nothing is
written where a web server could serve it. On later runs the CLI matches the
directory to its project and does not ask you to pick one again.

To accept everything without prompts, for example on a first automated run:

```
stackdrift scan --yes
```

This needs the project to be chosen once interactively in that directory first.

## Check for CVEs in CI

```
stackdrift check
```

This prints the CVE status of the project and exits with a non-zero code if any
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
- .NET Core Runtime, from a Dockerfile base image
- Laravel, from `composer.json`
- WordPress, from `wp-includes/version.php`
- The host operating system, from `/etc/os-release`
- The Linux kernel version
- Operating systems named in a Dockerfile `FROM` line

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
stackdrift remove      remove technologies or dependencies from the project
stackdrift whoami      show the signed in account
stackdrift update      download and install the latest release
stackdrift completion  print a shell completion script
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
