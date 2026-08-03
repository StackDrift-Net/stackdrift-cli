package detect

import (
	"regexp"
	"strconv"
	"strings"
)

// What the CurrentVersion registry key answers with. Kept apart from the read
// itself so the mapping is testable off Windows
type windowsInfo struct {
	InstallationType string
	ProductName      string
	DisplayVersion   string
	ReleaseId        string
	Build            int
}

// The first Windows 11 build. ProductName still says Windows 10 on many 11
// machines, so the build is the only trustworthy way to tell them apart
const firstWindows11Build = 22000

var serverReleaseRe = regexp.MustCompile(`(?i)(Windows Server \d{4}(?: R2)?)`)

// Server builds that predate a readable ProductName, or where it was blank
var serverBuilds = map[int]string{
	7601:  "Windows Server 2008 R2",
	9200:  "Windows Server 2012",
	9600:  "Windows Server 2012 R2",
	14393: "Windows Server 2016",
	17763: "Windows Server 2019",
	20348: "Windows Server 2022",
	26100: "Windows Server 2025",
}

var clientBuilds = map[int]string{
	7601: "Windows 7 - SP1",
	9200: "Windows 8",
	9600: "Windows 8.1",
}

// windowsTechnology maps the registry onto the catalog entry the release
// belongs to. Windows Server is its own technology, so a server must not be
// filed as a desktop release. An empty version means nothing matched the
// catalog and the user picks it, which beats storing a release that does not
// exist
func windowsTechnology(info windowsInfo) (string, string) {
	if isWindowsServer(info) {
		return "Windows Server", serverRelease(info)
	}

	if release, known := clientBuilds[info.Build]; known {
		return "Windows", release
	}

	major := clientMajor(info.Build)
	label := releaseLabel(info)
	if major == "" || label == "" {
		return "Windows", ""
	}
	return "Windows", major + " - " + label
}

func isWindowsServer(info windowsInfo) bool {
	return strings.Contains(strings.ToLower(info.InstallationType), "server") ||
		strings.Contains(strings.ToLower(info.ProductName), "windows server")
}

func serverRelease(info windowsInfo) string {
	if match := serverReleaseRe.FindStringSubmatch(info.ProductName); match != nil {
		return canonicalServer(match[1])
	}
	return serverBuilds[info.Build]
}

// ProductName casing is not guaranteed, and the catalog carries one spelling
func canonicalServer(raw string) string {
	parts := strings.Fields(strings.ToLower(raw))
	for i, part := range parts {
		switch part {
		case "windows":
			parts[i] = "Windows"
		case "server":
			parts[i] = "Server"
		case "r2":
			parts[i] = "R2"
		}
	}
	return strings.Join(parts, " ")
}

func clientMajor(build int) string {
	switch {
	case build >= firstWindows11Build:
		return "Windows 11"
	case build >= 10240:
		return "Windows 10"
	default:
		return ""
	}
}

// DisplayVersion only exists from 20H2 on, so anything older answers with
// ReleaseId instead
func releaseLabel(info windowsInfo) string {
	if label := strings.TrimSpace(info.DisplayVersion); label != "" {
		return label
	}
	return strings.TrimSpace(info.ReleaseId)
}

func atoi(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}
