package detect

import (
	"bufio"
	"os"
	"regexp"
	"runtime"
	"strings"
)

var distroNames = map[string]string{
	"ubuntu":        "Ubuntu",
	"debian":        "Debian",
	"fedora":        "Fedora",
	"rhel":          "Red Hat Enterprise Linux",
	"opensuse-leap": "openSUSE Leap",
	"alpine":        "Alpine Linux",
	"linuxmint":     "Linux Mint",
}

var kernelVersionRe = regexp.MustCompile(`^(\d+\.\d+)`)

func scanHost(result *Result) {
	switch runtime.GOOS {
	case "linux":
		detectOsRelease(result, "/etc/os-release")
		detectKernel(result)
		attachRunningKernel(result, runningKernel())
	case "windows":
		name, version := windowsTechnology(readWindowsInfo())
		result.Technologies = append(result.Technologies, Technology{
			Name:     name,
			Version:  version,
			Category: "OperatingSystem",
			Source:   SourceHost,
		})
	}

	// Runs on every platform, and after the operating system so the list reads
	// machine first. .NET installs the same way everywhere.
	scanDotNetHost(result)
}

// attachRunningKernel records the build the machine is actually booted into on
// the distribution entry, which is where StackDrift tracks a distro kernel: the
// distribution services it, so its line and build belong to that release rather
// than to upstream. The distro entry is the only one that gets it, since
// upstream point releases and a distribution's ABI counter are different
// numbers that happen to look alike.
func attachRunningKernel(result *Result, build string) {
	if build == "" {
		return
	}
	for i := range result.Technologies {
		if result.Technologies[i].Source == SourceOsRelease {
			result.Technologies[i].Kernel = build
		}
	}
}

func runningKernel() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return KernelCore(strings.TrimSpace(string(data)))
}

// KernelCore reduces a uname release to the version and ABI counter the server
// compares on, so "6.8.0-136-generic" and the archive's "6.8.0-136.136" agree.
// A release with no numeric ABI part, such as Debian's "6.12.94+deb13-amd64",
// keeps only the version.
func KernelCore(release string) string {
	if release == "" {
		return ""
	}
	parts := strings.Split(release, "-")
	if len(parts) < 2 {
		return parts[0]
	}
	abi := leadingDigits(parts[1])
	if abi == "" {
		return parts[0]
	}
	return parts[0] + "-" + abi
}

func leadingDigits(value string) string {
	for i, r := range value {
		if r < '0' || r > '9' {
			return value[:i]
		}
	}
	return value
}

func detectOsRelease(result *Result, path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	fields := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		fields[key] = strings.Trim(value, `"`)
	}

	name, known := distroNames[fields["ID"]]
	if !known {
		return
	}

	result.Technologies = append(result.Technologies, Technology{
		Name:     name,
		Version:  fields["VERSION_ID"],
		Category: "OperatingSystem",
		Source:   SourceOsRelease,
	})
}

func detectKernel(result *Result) {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return
	}
	match := kernelVersionRe.FindString(strings.TrimSpace(string(data)))
	if match == "" {
		return
	}
	result.Technologies = append(result.Technologies, Technology{
		Name:     "Linux Kernel",
		Version:  match,
		Category: "OperatingSystem",
		Source:   SourceHostKern,
	})
}
