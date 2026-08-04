package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeDirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(path)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func techsNamed(techs []Technology, name string) []Technology {
	var out []Technology
	for _, tech := range techs {
		if tech.Name == name {
			out = append(out, tech)
		}
	}
	return out
}

func TestDotNetHost_SdkAndRuntimeInstalls_AreBothReported(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/8.0.404", "shared/Microsoft.NETCore.App/8.0.11")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	sdk, ok := findTech(result.Technologies, ".NET Core SDK")
	if !ok {
		t.Fatalf("expected the installed SDK reported, got %+v", result.Technologies)
	}
	if sdk.Version != "8.0" || sdk.Kernel != "8.0.404" {
		t.Fatalf("expected 8.0 build 8.0.404, got %q build %q", sdk.Version, sdk.Kernel)
	}

	runtime, ok := findTech(result.Technologies, ".NET Core Runtime")
	if !ok {
		t.Fatalf("expected the installed runtime reported, got %+v", result.Technologies)
	}
	if runtime.Version != "8.0" || runtime.Kernel != "8.0.11" {
		t.Fatalf("expected 8.0 build 8.0.11, got %q build %q", runtime.Version, runtime.Kernel)
	}
}

func TestDotNetHost_SeveralLines_AreEachReported(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root,
		"shared/Microsoft.NETCore.App/6.0.36",
		"shared/Microsoft.NETCore.App/8.0.11",
		"shared/Microsoft.NETCore.App/9.0.2")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	runtimes := techsNamed(result.Technologies, ".NET Core Runtime")
	if len(runtimes) != 3 {
		t.Fatalf("expected all three runtime lines, got %+v", runtimes)
	}
}

func TestDotNetHost_SeveralPatchesOfOneLine_ReportOnlyTheNewestBuild(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root,
		"shared/Microsoft.NETCore.App/8.0.9",
		"shared/Microsoft.NETCore.App/8.0.14",
		"shared/Microsoft.NETCore.App/8.0.11")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	runtimes := techsNamed(result.Technologies, ".NET Core Runtime")
	if len(runtimes) != 1 {
		t.Fatalf("expected one row for the line, got %+v", runtimes)
	}
	// The newest patch is the one an app actually rolls forward onto, so it is
	// the build the line is running.
	if runtimes[0].Kernel != "8.0.14" {
		t.Fatalf("expected the newest build 8.0.14, got %q", runtimes[0].Kernel)
	}
}

func TestDotNetHost_LinesAreReportedNewestFirst(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root,
		"sdk/6.0.428",
		"sdk/10.0.100",
		"sdk/8.0.404")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	sdks := techsNamed(result.Technologies, ".NET Core SDK")
	if len(sdks) != 3 {
		t.Fatalf("expected three SDK lines, got %+v", sdks)
	}
	if sdks[0].Version != "10.0" || sdks[1].Version != "8.0" || sdks[2].Version != "6.0" {
		t.Fatalf("expected 10.0, 8.0, 6.0 in that order, got %+v", sdks)
	}
}

func TestDotNetHost_NonVersionDirectories_AreIgnored(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/NuGetFallbackFolder", "sdk/8.0.404")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	sdks := techsNamed(result.Technologies, ".NET Core SDK")
	if len(sdks) != 1 {
		t.Fatalf("expected only the version directory read, got %+v", sdks)
	}
}

func TestDotNetHost_FileWhereAVersionDirectoryBelongs_IsIgnored(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk")
	write(t, root, "sdk/8.0.404", "not a directory")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	if len(techsNamed(result.Technologies, ".NET Core SDK")) != 0 {
		t.Fatalf("expected nothing from a file, got %+v", result.Technologies)
	}
}

func TestDotNetHost_MissingRoot_ReportsNothing(t *testing.T) {
	result := &Result{}
	appendDotNetHost(result, []string{filepath.Join(t.TempDir(), "nothing-here")})

	if len(result.Technologies) != 0 {
		t.Fatalf("expected nothing from a root that does not exist, got %+v", result.Technologies)
	}
}

func TestDotNetHost_SourceNamesTheInstallAndCountsAsHost(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "shared/Microsoft.NETCore.App/8.0.11")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	tech, ok := findTech(result.Technologies, ".NET Core Runtime")
	if !ok {
		t.Fatal("expected the runtime reported")
	}
	if !strings.Contains(tech.Source, root) {
		t.Fatalf("expected the source to name the install root, got %q", tech.Source)
	}
	// An install on the machine describes the machine, not the directory being
	// scanned, so it must stay unticked until somebody says otherwise.
	if !IsHostSource(tech.Source) {
		t.Fatalf("expected a host source, got %q", tech.Source)
	}
}

// ASP.NET Core ships in the same release as the runtime under it and cannot be
// installed on its own, so reading it as well would report one line twice.
func TestDotNetHost_AspNetCoreSharedFramework_IsNotReportedOnItsOwn(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root,
		"shared/Microsoft.AspNetCore.App/8.0.11",
		"shared/Microsoft.WindowsDesktop.App/8.0.11",
		"shared/Microsoft.NETCore.App/8.0.11")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	runtimes := techsNamed(result.Technologies, ".NET Core Runtime")
	if len(runtimes) != 1 {
		t.Fatalf("expected one runtime row, got %+v", runtimes)
	}
}

func TestDotNetHost_PreviewSdk_LosesToTheStableBuildOfTheSameLine(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/10.0.100-preview.7.25380.108", "sdk/10.0.100")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	sdk, _ := findTech(result.Technologies, ".NET Core SDK")
	if sdk.Kernel != "10.0.100" {
		t.Fatalf("expected the stable build to win, got %q", sdk.Kernel)
	}
}

func TestDotNetHost_OnlyAPreviewInstalled_IsStillReported(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/10.0.100-preview.7.25380.108")

	result := &Result{}
	appendDotNetHost(result, []string{root})

	sdk, ok := findTech(result.Technologies, ".NET Core SDK")
	if !ok {
		t.Fatalf("expected the preview reported, got %+v", result.Technologies)
	}
	if sdk.Version != "10.0" || sdk.Kernel != "10.0.100-preview.7.25380.108" {
		t.Fatalf("expected 10.0 with the preview build, got %q build %q", sdk.Version, sdk.Kernel)
	}
}

// Two install roots holding the same line is ordinary: a distribution package
// and a hand installed copy under the home directory. The newest build is the
// one reported, and the row names the root it came from.
func TestDotNetHost_TwoRootsOfOneLine_ReportTheNewestBuild(t *testing.T) {
	older := t.TempDir()
	newer := t.TempDir()
	makeDirs(t, older, "shared/Microsoft.NETCore.App/8.0.11")
	makeDirs(t, newer, "shared/Microsoft.NETCore.App/8.0.14")

	result := &Result{}
	appendDotNetHost(result, []string{older, newer})

	runtimes := techsNamed(result.Technologies, ".NET Core Runtime")
	if len(runtimes) != 1 {
		t.Fatalf("expected one row for the line, got %+v", runtimes)
	}
	if runtimes[0].Kernel != "8.0.14" {
		t.Fatalf("expected 8.0.14, got %q", runtimes[0].Kernel)
	}
	if !strings.Contains(runtimes[0].Source, newer) {
		t.Fatalf("expected the source to name the root holding it, got %q", runtimes[0].Source)
	}
}

func TestDotNetHost_RepeatedRoot_IsReadOnce(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/8.0.404")

	result := &Result{}
	appendDotNetHost(result, []string{root, root})

	if len(techsNamed(result.Technologies, ".NET Core SDK")) != 1 {
		t.Fatalf("expected one SDK row, got %+v", result.Technologies)
	}
}

func TestDotNetRoots_DotnetRootEnvironmentVariable_IsSearched(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "sdk/8.0.404")
	t.Setenv("DOTNET_ROOT", root)

	if !containsPath(dotNetRoots(), root) {
		t.Fatalf("expected DOTNET_ROOT searched, got %v", dotNetRoots())
	}
}

// The file the .NET host itself reads to find an install that is not in any of
// the usual places.
func TestDotNetRoots_InstallLocationFile_IsSearched(t *testing.T) {
	install := t.TempDir()
	etc := t.TempDir()
	write(t, etc, "install_location", install+"\n")

	original := dotNetInstallLocationDir
	t.Cleanup(func() { dotNetInstallLocationDir = original })
	dotNetInstallLocationDir = etc

	if !containsPath(dotNetRoots(), install) {
		t.Fatalf("expected the recorded install location searched, got %v", dotNetRoots())
	}
}

func TestDotNetRoots_ArchitectureSpecificInstallLocation_IsSearched(t *testing.T) {
	install := t.TempDir()
	etc := t.TempDir()
	write(t, etc, "install_location_x64", install+"\n")

	original := dotNetInstallLocationDir
	t.Cleanup(func() { dotNetInstallLocationDir = original })
	dotNetInstallLocationDir = etc

	if !containsPath(dotNetRoots(), install) {
		t.Fatalf("expected the architecture specific install location searched, got %v", dotNetRoots())
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == filepath.Clean(want) {
			return true
		}
	}
	return false
}

func TestDotNetLine_SplitsOnMajorAndMinor(t *testing.T) {
	cases := map[string]string{
		"8.0.404":                    "8.0",
		"8.0":                        "8.0",
		"10.0.100-preview.7.25380.1": "10.0",
		"3.1.32":                     "3.1",
		"6":                          "6",
	}
	for version, want := range cases {
		if got := dotNetLine(version); got != want {
			t.Fatalf("dotNetLine(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestCompareBuilds_ComparesNumericallyNotAsText(t *testing.T) {
	if compareBuilds("8.0.9", "8.0.14") >= 0 {
		t.Fatal("expected 8.0.9 to be older than 8.0.14")
	}
	if compareBuilds("10.0.100", "9.0.100") <= 0 {
		t.Fatal("expected 10.0.100 to be newer than 9.0.100")
	}
	if compareBuilds("8.0.11", "8.0.11") != 0 {
		t.Fatal("expected equal builds to compare equal")
	}
}

func TestCompareBuilds_PrereleaseIsOlderThanItsRelease(t *testing.T) {
	if compareBuilds("10.0.100-preview.7", "10.0.100") >= 0 {
		t.Fatal("expected a preview to be older than the release it leads to")
	}
	if compareBuilds("10.0.100-preview.7", "10.0.100-preview.2") <= 0 {
		t.Fatal("expected the later preview to win")
	}
}

// Proves the host scan is wired into a plain scan, using a version no real
// machine has so the result cannot come from the machine running the test.
func TestScan_ReportsDotNetInstalledOnTheMachine(t *testing.T) {
	root := t.TempDir()
	makeDirs(t, root, "shared/Microsoft.NETCore.App/42.1.5")
	t.Setenv("DOTNET_ROOT", root)

	result, err := Scan(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, tech := range techsNamed(result.Technologies, ".NET Core Runtime") {
		if tech.Version == "42.1" && tech.Kernel == "42.1.5" {
			return
		}
	}
	t.Fatalf("expected the installed runtime in a plain scan, got %+v", result.Technologies)
}
