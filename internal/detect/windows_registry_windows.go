//go:build windows

package detect

import "golang.org/x/sys/windows/registry"

const currentVersionKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`

// A missing value is normal rather than an error: DisplayVersion does not exist
// before 20H2 and ReleaseId is gone after it, so each is read on its own
func readWindowsInfo() windowsInfo {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return windowsInfo{}
	}
	defer key.Close()

	info := windowsInfo{
		InstallationType: stringValue(key, "InstallationType"),
		ProductName:      stringValue(key, "ProductName"),
		DisplayVersion:   stringValue(key, "DisplayVersion"),
		ReleaseId:        stringValue(key, "ReleaseId"),
	}

	// CurrentBuildNumber is a string on every version that matters, and
	// CurrentBuild is the older spelling of the same thing
	if build := stringValue(key, "CurrentBuildNumber"); build != "" {
		info.Build = atoi(build)
	}
	if info.Build == 0 {
		info.Build = atoi(stringValue(key, "CurrentBuild"))
	}
	return info
}

func stringValue(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}
