package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// .NET installs releases side by side and never removes the old ones, so what a
// machine is exposed to is the set of SDKs and runtimes sitting on disk rather
// than whatever the project in front of it happens to target. The catalog
// tracks the two separately because they are patched separately and carry
// different advisories, and the layout is the same on every platform: versioned
// directories under sdk and under shared.
const dotNetSdkDir = "sdk"

// Only the base runtime is read. ASP.NET Core and Windows Desktop are layers on
// top of it, ship in the same release, and cannot be installed without it, so
// reading them as well would report one line several times over.
var dotNetRuntimeDir = filepath.Join("shared", "Microsoft.NETCore.App")

// Where the .NET host itself records an install that is in none of the usual
// places. A variable so a test can point it somewhere writable.
var dotNetInstallLocationDir = "/etc/dotnet"

// Anchored so a directory that is not a version, such as the SDK's
// NuGetFallbackFolder, is never read as one. The tail is deliberately loose:
// previews carry a suffix like 10.0.100-preview.7.25380.108.
var dotNetVersionDirRe = regexp.MustCompile(`^\d+(?:\.\d+)*(?:[-+][0-9A-Za-z.+-]+)?$`)

// dotNetInstall is one version directory and the root it was found under, so
// the row can name the install that supplied the build it reports.
type dotNetInstall struct {
	Version string
	Root    string
}

func scanDotNetHost(result *Result) {
	appendDotNetHost(result, dotNetRoots())
}

func appendDotNetHost(result *Result, roots []string) {
	roots = uniquePaths(roots)
	appendDotNetLines(result, ".NET Core SDK", installsUnder(roots, dotNetSdkDir))
	appendDotNetLines(result, ".NET Core Runtime", installsUnder(roots, dotNetRuntimeDir))
}

// appendDotNetLines reports one row per release line. Older patches of a line
// are left out on purpose: an app rolls forward onto the newest patch installed,
// so that is the build the line is running, and the older directories beside it
// are inert.
func appendDotNetLines(result *Result, name string, found []dotNetInstall) {
	for _, install := range newestPerLine(found) {
		result.Technologies = append(result.Technologies, Technology{
			Name:    name,
			Version: dotNetLine(install.Version),
			// The line carries the support dates and the build says which patch
			// is installed, which is the same split WordPress and the distros
			// use. Only this detector can supply the build.
			Kernel:   install.Version,
			Category: "Framework",
			Source:   SourceHostPrefix + install.Root,
		})
	}
}

func installsUnder(roots []string, subdir string) []dotNetInstall {
	var found []dotNetInstall
	for _, root := range roots {
		for _, version := range versionDirs(filepath.Join(root, subdir)) {
			found = append(found, dotNetInstall{Version: version, Root: root})
		}
	}
	return found
}

func versionDirs(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}

	var versions []string
	for _, entry := range entries {
		name := entry.Name()
		if !dotNetVersionDirRe.MatchString(name) {
			continue
		}
		// Stat rather than the entry's own answer, so an install reached
		// through a symlinked version directory is still read.
		if !entry.IsDir() && !isDir(filepath.Join(path, name)) {
			continue
		}
		versions = append(versions, name)
	}
	return versions
}

// newestPerLine keeps the highest build of each line, newest line first, so the
// list reads the way somebody would want to act on it.
func newestPerLine(found []dotNetInstall) []dotNetInstall {
	best := map[string]dotNetInstall{}
	for _, install := range found {
		line := dotNetLine(install.Version)
		current, seen := best[line]
		if !seen || compareBuilds(install.Version, current.Version) > 0 {
			best[line] = install
		}
	}

	out := make([]dotNetInstall, 0, len(best))
	for _, install := range best {
		out = append(out, install)
	}
	sort.Slice(out, func(i, j int) bool {
		return compareBuilds(dotNetLine(out[i].Version), dotNetLine(out[j].Version)) > 0
	})
	return out
}

// dotNetLine reduces a version to the release line the catalog tracks, which
// for .NET is always major.minor. It is a starting point that the scan then
// resolves against the catalog, which is the only thing that knows the real
// granularity.
func dotNetLine(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// compareBuilds orders two .NET versions. Numeric parts compare as numbers, so
// 8.0.9 is older than 8.0.14 rather than newer the way text sorting would have
// it, and a prerelease sits below the release it leads to.
func compareBuilds(a, b string) int {
	releaseA, prereleaseA := splitPrerelease(a)
	releaseB, prereleaseB := splitPrerelease(b)

	if order := compareNumericParts(releaseA, releaseB); order != 0 {
		return order
	}

	switch {
	case prereleaseA == prereleaseB:
		return 0
	case prereleaseA == "":
		return 1
	case prereleaseB == "":
		return -1
	}
	return strings.Compare(prereleaseA, prereleaseB)
}

func splitPrerelease(version string) (release, prerelease string) {
	if at := strings.IndexAny(version, "-+"); at >= 0 {
		return version[:at], version[at+1:]
	}
	return version, ""
}

func compareNumericParts(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	for i := 0; i < len(partsA) || i < len(partsB); i++ {
		if order := numberAt(partsA, i) - numberAt(partsB, i); order != 0 {
			if order < 0 {
				return -1
			}
			return 1
		}
	}
	return 0
}

func numberAt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return value
}

// dotNetRoots lists every place a .NET install could be, most specific first:
// what the environment says, then what the host recorded, then whatever the
// dotnet on PATH resolves to, then the packaged locations. A root that does not
// exist costs one failed directory read.
func dotNetRoots() []string {
	var roots []string

	for _, key := range []string{"DOTNET_ROOT", "DOTNET_ROOT(x86)", "DOTNET_ROOT_X64", "DOTNET_ROOT_X86"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			roots = append(roots, value)
		}
	}

	roots = append(roots, recordedInstallLocations()...)

	if root, found := dotNetRootOnPath(); found {
		roots = append(roots, root)
	}

	return uniquePaths(append(roots, packagedDotNetRoots()...))
}

// The install_location files are how the .NET host finds a root that is not in
// a standard place, which is the case on any distribution that packages dotnet
// itself. Each holds a single path, one per architecture.
func recordedInstallLocations() []string {
	entries, err := os.ReadDir(dotNetInstallLocationDir)
	if err != nil {
		return nil
	}

	var roots []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "install_location") {
			continue
		}
		content, ok := readCapped(filepath.Join(dotNetInstallLocationDir, entry.Name()))
		if !ok {
			continue
		}
		if path := strings.TrimSpace(content); path != "" {
			roots = append(roots, path)
		}
	}
	return roots
}

// The muxer sits in the install root, so resolving whichever dotnet is on PATH
// finds an install nothing else knows about. Nothing is executed: the path is
// read and the symlinks followed.
func dotNetRootOnPath() (string, bool) {
	path, err := exec.LookPath("dotnet")
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Dir(path), true
}

func packagedDotNetRoots() []string {
	var roots []string
	home, err := os.UserHomeDir()
	if err == nil {
		roots = append(roots, filepath.Join(home, ".dotnet"))
	}

	switch runtime.GOOS {
	case "windows":
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if value := os.Getenv(key); value != "" {
				roots = append(roots, filepath.Join(value, "dotnet"))
			}
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			roots = append(roots, filepath.Join(local, "Microsoft", "dotnet"))
		}
	case "darwin":
		roots = append(roots,
			"/usr/local/share/dotnet",
			"/usr/local/share/dotnet/x64",
			"/opt/homebrew/opt/dotnet/libexec")
	default:
		// Microsoft's packages install to /usr/share/dotnet and a distribution's
		// own to /usr/lib/dotnet, and both are ordinary on the same machine.
		roots = append(roots,
			"/usr/share/dotnet",
			"/usr/lib/dotnet",
			"/usr/local/share/dotnet",
			"/opt/dotnet",
			"/snap/dotnet-sdk/current")
	}
	return roots
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if clean == "" || clean == "." || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}
