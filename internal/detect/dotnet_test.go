package detect

import (
	"path/filepath"
	"testing"
)

// The machine running the tests has its own .NET installs, which a plain scan
// reports as well. These helpers keep an assertion about a project to what the
// project itself said.
func projectTechs(result *Result, name string) []Technology {
	var out []Technology
	for _, tech := range techsNamed(result.Technologies, name) {
		if !IsHostSource(tech.Source) {
			out = append(out, tech)
		}
	}
	return out
}

func dockerfileTechs(t *testing.T, content string) []Technology {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "Dockerfile", content)

	result := &Result{}
	detectDockerfile(result, filepath.Join(dir, "Dockerfile"))
	return result.Technologies
}

func TestScan_GlobalJson_DetectsThePinnedSdkBuild(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "global.json", `{"sdk":{"version":"8.0.404","rollForward":"latestFeature"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	sdk, ok := findTech(result.Technologies, ".NET Core SDK")
	if !ok {
		t.Fatalf("expected the pinned SDK reported, got %+v", result.Technologies)
	}
	if sdk.Version != "8.0" || sdk.Kernel != "8.0.404" {
		t.Fatalf("expected 8.0 build 8.0.404, got %q build %q", sdk.Version, sdk.Kernel)
	}
	if IsHostSource(sdk.Source) {
		t.Fatalf("global.json belongs to the project, not the machine, got source %q", sdk.Source)
	}
}

func TestScan_GlobalJsonWithoutAnSdkVersion_DetectsNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "global.json", `{"msbuild-sdks":{"Microsoft.Build.Traversal":"3.4.0"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, tech := range result.Technologies {
		if tech.Source == "global.json" {
			t.Fatalf("expected nothing from a global.json with no SDK version, got %+v", tech)
		}
	}
}

func TestScan_GlobalJsonWithAnUnusableVersion_IsIgnored(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "global.json", `{"sdk":{"version":"latest"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, tech := range result.Technologies {
		if tech.Source == "global.json" {
			t.Fatalf("expected an unusable version ignored, got %+v", tech)
		}
	}
}

// The csproj knows the line and global.json knows the build, so the two have to
// land on one row carrying both rather than two rows for the same install.
func TestScan_GlobalJsonBesideACsproj_ProducesOneRowCarryingTheBuild(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.csproj", "<Project><TargetFramework>net8.0</TargetFramework></Project>")
	write(t, dir, "global.json", `{"sdk":{"version":"8.0.404"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	sdks := projectTechs(result, ".NET Core SDK")
	if len(sdks) != 1 {
		t.Fatalf("expected one SDK row, got %+v", sdks)
	}
	if sdks[0].Kernel != "8.0.404" {
		t.Fatalf("expected the build kept, got %q", sdks[0].Kernel)
	}
}

func TestDockerfile_SdkImage_DetectsTheSdkNotTheRuntime(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build")

	sdk, ok := findTech(techs, ".NET Core SDK")
	if !ok {
		t.Fatalf("expected an sdk image reported as the SDK, got %+v", techs)
	}
	if sdk.Version != "9.0" {
		t.Fatalf("expected 9.0, got %q", sdk.Version)
	}
}

func TestDockerfile_BuildAndRuntimeStages_DetectBoth(t *testing.T) {
	techs := dockerfileTechs(t,
		"FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build\n"+
			"FROM mcr.microsoft.com/dotnet/aspnet:8.0 AS final\n")

	if _, ok := findTech(techs, ".NET Core SDK"); !ok {
		t.Fatalf("expected the build stage SDK, got %+v", techs)
	}
	if _, ok := findTech(techs, ".NET Core Runtime"); !ok {
		t.Fatalf("expected the final stage runtime, got %+v", techs)
	}
}

func TestDockerfile_NightlySdkImage_DetectsTheSdk(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/nightly/sdk:10.0")

	if _, ok := findTech(techs, ".NET Core SDK"); !ok {
		t.Fatalf("expected the nightly sdk image reported as the SDK, got %+v", techs)
	}
}

func TestDockerfile_RuntimeDepsImage_DetectsTheRuntime(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/runtime-deps:8.0-alpine")

	if _, ok := findTech(techs, ".NET Core Runtime"); !ok {
		t.Fatalf("expected the runtime, got %+v", techs)
	}
}

// An image pinned to a patch names the exact build, which is worth keeping:
// the line alone would be scored against the newest patch of that line.
func TestDockerfile_ImagePinnedToAPatch_KeepsTheBuild(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/aspnet:8.0.11-alpine")

	tech, ok := findTech(techs, ".NET Core Runtime")
	if !ok {
		t.Fatalf("expected the runtime, got %+v", techs)
	}
	if tech.Version != "8.0" || tech.Kernel != "8.0.11" {
		t.Fatalf("expected 8.0 build 8.0.11, got %q build %q", tech.Version, tech.Kernel)
	}
}

func TestDockerfile_ImagePinnedToALine_NamesNoBuild(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/aspnet:8.0")

	tech, _ := findTech(techs, ".NET Core Runtime")
	if tech.Kernel != "" {
		t.Fatalf("a line tag names no build, got %q", tech.Kernel)
	}
}

// The Windows containers are Full Framework, not .NET Core, and filing them as
// a Core runtime tracked a release that does not exist.
func TestDockerfile_FrameworkImage_DetectsFullFramework(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/framework/aspnet:4.8")

	tech, ok := findTech(techs, ".NET Full Framework")
	if !ok {
		t.Fatalf("expected Full Framework, got %+v", techs)
	}
	if tech.Version != "4.8" {
		t.Fatalf("expected 4.8, got %q", tech.Version)
	}
	if _, ok := findTech(techs, ".NET Core Runtime"); ok {
		t.Fatal("a Full Framework image must not be filed as a Core runtime")
	}
}

// The catalog carries 4.8.1 as its own release, so it is the version rather
// than a build of 4.8.
func TestDockerfile_FrameworkSdkImage_KeepsTheWholeVersion(t *testing.T) {
	techs := dockerfileTechs(t, "FROM mcr.microsoft.com/dotnet/framework/sdk:4.8.1")

	tech, ok := findTech(techs, ".NET Full Framework")
	if !ok {
		t.Fatalf("expected Full Framework, got %+v", techs)
	}
	if tech.Version != "4.8.1" || tech.Kernel != "" {
		t.Fatalf("expected 4.8.1 with no build, got %q build %q", tech.Version, tech.Kernel)
	}
	if _, ok := findTech(techs, ".NET Core SDK"); ok {
		t.Fatal("a Full Framework sdk image must not be filed as a Core SDK")
	}
}
