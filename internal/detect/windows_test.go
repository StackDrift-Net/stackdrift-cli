package detect

import "testing"

func TestWindowsTechnology_Eleven_ReadsTheReleaseFromDisplayVersion(t *testing.T) {
	name, version := windowsTechnology(windowsInfo{
		InstallationType: "Client",
		ProductName:      "Windows 10 Pro",
		DisplayVersion:   "24H2",
		Build:            26100,
	})

	if name != "Windows" || version != "Windows 11 - 24H2" {
		t.Fatalf("got %q %q", name, version)
	}
}

// ProductName still reads "Windows 10" on a Windows 11 machine, which is why
// the build decides the major and the name is never trusted for it
func TestWindowsTechnology_ElevenReportingItselfAsTen_FollowsTheBuild(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{
		ProductName:    "Windows 10 Enterprise",
		DisplayVersion: "23H2",
		Build:          22631,
	})

	if version != "Windows 11 - 23H2" {
		t.Fatalf("got %q", version)
	}
}

func TestWindowsTechnology_Ten_ReadsTheReleaseFromDisplayVersion(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{DisplayVersion: "22H2", Build: 19045})

	if version != "Windows 10 - 22H2" {
		t.Fatalf("got %q", version)
	}
}

// DisplayVersion only exists from 20H2 on, so anything older answers with
// ReleaseId instead
func TestWindowsTechnology_TenBefore20H2_FallsBackToReleaseId(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{ReleaseId: "1909", Build: 18363})

	if version != "Windows 10 - 1909" {
		t.Fatalf("got %q", version)
	}
}

func TestWindowsTechnology_FirstElevenBuild_IsNotReportedAsTen(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{DisplayVersion: "21H2", Build: 22000})

	if version != "Windows 11 - 21H2" {
		t.Fatalf("22000 is the first Windows 11 build, got %q", version)
	}
}

func TestWindowsTechnology_LastTenBuild_IsStillTen(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{DisplayVersion: "21H2", Build: 21999})

	if version != "Windows 10 - 21H2" {
		t.Fatalf("got %q", version)
	}
}

// Windows Server is its own catalog technology, so a server must not be filed
// as a desktop release
func TestWindowsTechnology_Server_UsesTheServerCatalogEntry(t *testing.T) {
	name, version := windowsTechnology(windowsInfo{
		InstallationType: "Server",
		ProductName:      "Windows Server 2022 Datacenter",
		DisplayVersion:   "21H2",
		Build:            20348,
	})

	if name != "Windows Server" || version != "Windows Server 2022" {
		t.Fatalf("got %q %q", name, version)
	}
}

func TestWindowsTechnology_ServerR2_KeepsTheR2(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{
		InstallationType: "Server",
		ProductName:      "Windows Server 2012 R2 Standard",
		Build:            9600,
	})

	if version != "Windows Server 2012 R2" {
		t.Fatalf("R2 is a separate catalog release, got %q", version)
	}
}

func TestWindowsTechnology_ServerCore_IsStillAServer(t *testing.T) {
	name, _ := windowsTechnology(windowsInfo{
		InstallationType: "Server Core",
		ProductName:      "Windows Server 2019 Standard",
		Build:            17763,
	})

	if name != "Windows Server" {
		t.Fatalf("got %q", name)
	}
}

func TestWindowsTechnology_ServerWithAnUnhelpfulProductName_FallsBackToTheBuild(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{InstallationType: "Server", Build: 26100})

	if version != "Windows Server 2025" {
		t.Fatalf("got %q", version)
	}
}

func TestWindowsTechnology_EightPointOne_IsNamedWithoutARelease(t *testing.T) {
	_, version := windowsTechnology(windowsInfo{ProductName: "Windows 8.1 Pro", Build: 9600})

	if version != "Windows 8.1" {
		t.Fatalf("got %q", version)
	}
}

// Better an unset version the user picks from the dropdown than a string the
// catalog does not carry, which would read as an unknown release forever
func TestWindowsTechnology_NoReleaseLabel_LeavesTheVersionUnset(t *testing.T) {
	name, version := windowsTechnology(windowsInfo{Build: 26100})

	if name != "Windows" || version != "" {
		t.Fatalf("got %q %q", name, version)
	}
}

func TestWindowsTechnology_NothingReadable_StillReportsWindows(t *testing.T) {
	name, version := windowsTechnology(windowsInfo{})

	if name != "Windows" || version != "" {
		t.Fatalf("got %q %q", name, version)
	}
}
