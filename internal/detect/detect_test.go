package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findTech(techs []Technology, name string) (Technology, bool) {
	for _, tech := range techs {
		if tech.Name == name {
			return tech, true
		}
	}
	return Technology{}, false
}

func hasManifest(manifests []Manifest, fileName, ecosystem string) bool {
	for _, m := range manifests {
		if m.FileName == fileName && m.Ecosystem == ecosystem {
			return true
		}
	}
	return false
}

func TestScan_NetCoreCsproj_DetectsSdkAndNuGetManifest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.csproj", "<Project><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Core SDK")
	if !ok {
		t.Fatal("expected .NET Core SDK")
	}
	if tech.Version != "8.0" {
		t.Fatalf("expected version 8.0, got %q", tech.Version)
	}
	if !hasManifest(result.Manifests, "app.csproj", "NuGet") {
		t.Fatal("expected csproj as a NuGet manifest")
	}
}

func TestScan_FullFrameworkCsproj_DetectsFullFramework(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "legacy.csproj", "<Project><TargetFramework>net48</TargetFramework></Project>")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Full Framework")
	if !ok {
		t.Fatal("expected .NET Full Framework")
	}
	if tech.Version != "4.8" {
		t.Fatalf("expected 4.8, got %q", tech.Version)
	}
}

func TestScan_MultiTargetCsproj_DetectsBoth(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "multi.csproj", "<Project><TargetFrameworks>net48;net8.0</TargetFrameworks></Project>")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := findTech(result.Technologies, ".NET Full Framework"); !ok {
		t.Fatal("expected full framework")
	}
	if _, ok := findTech(result.Technologies, ".NET Core SDK"); !ok {
		t.Fatal("expected core sdk")
	}
}

func TestScan_ComposerWithLaravel_DetectsLaravelVersion(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.9"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, "Laravel")
	if !ok {
		t.Fatal("expected Laravel")
	}
	if tech.Version != "11.9" {
		t.Fatalf("expected 11.9, got %q", tech.Version)
	}
}

func TestScan_ComposerWithoutLaravel_DetectsNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "composer.json", `{"require":{"symfony/console":"^7.0"}}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := findTech(result.Technologies, "Laravel"); ok {
		t.Fatal("did not expect Laravel")
	}
}

func TestScan_NpmManifests_DetectedAsNpm(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"x"}`)
	write(t, dir, "package-lock.json", `{"lockfileVersion":3}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !hasManifest(result.Manifests, "package.json", "Npm") {
		t.Fatal("expected package.json npm")
	}
	if !hasManifest(result.Manifests, "package-lock.json", "Npm") {
		t.Fatal("expected package-lock.json npm")
	}
}

func TestScan_YarnAndPnpmLocks_DetectedAsNpm(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"x"}`)
	write(t, dir, "yarn.lock", "react@^18.0.0:\n  version \"18.3.1\"\n")
	write(t, dir, "web/package.json", `{"name":"y"}`)
	write(t, dir, "web/pnpm-lock.yaml", "lockfileVersion: '9.0'\n")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !hasManifest(result.Manifests, "yarn.lock", "Npm") {
		t.Fatal("expected yarn.lock npm")
	}
	if !hasManifest(result.Manifests, "pnpm-lock.yaml", "Npm") {
		t.Fatal("expected pnpm-lock.yaml npm")
	}
}

func TestScan_NodeModules_IsSkipped(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"root"}`)
	write(t, dir, "node_modules/dep/package.json", `{"name":"dep"}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, m := range result.Manifests {
		if m.FileName == "package.json" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 package.json (node_modules skipped), got %d", count)
	}
}

func TestScan_DockerfileUbuntu_DetectsUbuntu(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Dockerfile", "FROM ubuntu:24.04\nRUN echo hi")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, "Ubuntu")
	if !ok {
		t.Fatal("expected Ubuntu")
	}
	if tech.Version != "24.04" {
		t.Fatalf("expected 24.04, got %q", tech.Version)
	}
}

func TestScan_DockerfileDotnet_DetectsRuntime(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Dockerfile", "FROM mcr.microsoft.com/dotnet/aspnet:8.0")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Core Runtime")
	if !ok {
		t.Fatal("expected .NET Core Runtime")
	}
	if tech.Version != "8.0" {
		t.Fatalf("expected 8.0, got %q", tech.Version)
	}
}

func TestScan_PlatformTargetedTfm_DetectsSdk(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "wpf.csproj", "<Project><TargetFramework>net8.0-windows10.0.19041.0</TargetFramework></Project>")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Core SDK")
	if !ok {
		t.Fatal("expected .NET Core SDK for platform-targeted TFM")
	}
	if tech.Version != "8.0" {
		t.Fatalf("expected 8.0, got %q", tech.Version)
	}
	if tech.Category != "Framework" {
		t.Fatalf("expected Framework category, got %q", tech.Category)
	}
}

func TestScan_SuffixedDotnetImageTag_UsesNumericVersion(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Dockerfile", "FROM mcr.microsoft.com/dotnet/aspnet:8.0-alpine")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Core Runtime")
	if !ok {
		t.Fatal("expected .NET Core Runtime")
	}
	if tech.Version != "8.0" {
		t.Fatalf("expected clean version 8.0, got %q", tech.Version)
	}
}

func TestScan_DebianImageMajorTag_UsesMajor(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Dockerfile", "FROM debian:12-slim")

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, "Debian")
	if !ok {
		t.Fatal("expected Debian")
	}
	if tech.Version != "12" {
		t.Fatalf("expected 12, got %q", tech.Version)
	}
}

func TestIsHostSource(t *testing.T) {
	if !IsHostSource(SourceOsRelease) || !IsHostSource(SourceHostKern) || !IsHostSource(SourceHost) {
		t.Fatal("expected host sources to be recognised")
	}
	if IsHostSource("csproj TargetFramework") || IsHostSource("Dockerfile") {
		t.Fatal("project sources must not count as host")
	}
}

func TestScan_PackagesConfig_DetectedAsNuGetSupportingFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Caterex.Data.csproj", `<Project ToolsVersion="15.0"><PropertyGroup><TargetFrameworkVersion>v4.8</TargetFrameworkVersion></PropertyGroup></Project>`)
	write(t, dir, "packages.config", `<packages><package id="Newtonsoft.Json" version="13.0.3" /></packages>`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !hasManifest(result.Manifests, "packages.config", "NuGet") {
		t.Fatal("expected packages.config detected as a NuGet manifest")
	}
	for _, m := range result.Manifests {
		// It must not be primary, or an old style project would upload twice:
		// once as its csproj group and once as a group of its own.
		if m.FileName == "packages.config" && m.Primary {
			t.Fatal("packages.config must ride along with the csproj, not form its own group")
		}
	}
}

// Proves the callback fires before the walk: the manifest is only created from
// inside the callback, so the walk could not have seen it unless it ran after.
func TestScanWithProgressCallbackRunsBeforeTheTreeWalk(t *testing.T) {
	dir := t.TempDir()

	called := 0
	result, err := ScanWithProgress(dir, func() {
		called++
		write(t, dir, "package.json", `{"name":"x"}`)
	})
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("expected the progress callback once, got %d", called)
	}
	if !hasManifest(result.Manifests, "package.json", "Npm") {
		t.Fatal("the tree walk must run after the progress callback")
	}
}

func TestScanWithoutProgressCallbackStillScans(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"x"}`)

	result, err := ScanWithProgress(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasManifest(result.Manifests, "package.json", "Npm") {
		t.Fatal("expected package.json npm")
	}
}

// A technology found in the tree must keep winning over the same one detected on
// the host, because the source decides whether it defaults to selected. Host
// detection now runs first, so this guards the ordering that dedupe relies on.
func TestScanPrefersTreeSourceOverHostForTheSameTechnology(t *testing.T) {
	tree := []Technology{{Name: "Ubuntu", Version: "24.04", Source: "Dockerfile"}}
	host := []Technology{{Name: "Ubuntu", Version: "24.04", Source: SourceOsRelease}}

	merged := dedupeTechnologies(append(tree, host...))

	if len(merged) != 1 {
		t.Fatalf("expected one entry after dedupe, got %d", len(merged))
	}
	if merged[0].Source != "Dockerfile" {
		t.Fatalf("expected the tree source to win, got %q", merged[0].Source)
	}
	if IsHostSource(merged[0].Source) {
		t.Fatal("a tree detection must not be treated as a host detection")
	}
}

// The style every project with a packages.config uses. It declares
// TargetFrameworkVersion, not TargetFramework, so the SDK-style pattern never
// matched it and a whole Full Framework solution reported no framework at all
func TestScan_ClassicCsproj_DetectsFullFramework(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Caterex.Data.csproj", `<Project ToolsVersion="15.0"><PropertyGroup><TargetFrameworkVersion>v4.8</TargetFrameworkVersion></PropertyGroup></Project>`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, ok := findTech(result.Technologies, ".NET Full Framework")
	if !ok {
		t.Fatalf("expected .NET Full Framework, got %+v", result.Technologies)
	}
	if tech.Version != "4.8" {
		t.Fatalf("expected 4.8, got %q", tech.Version)
	}
}

// The catalog carries 4.5.1 and 4.7.2, so the third part cannot be dropped
func TestScan_ClassicCsprojWithAPatchVersion_KeepsAllThreeParts(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "legacy.csproj", `<Project><PropertyGroup><TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion></PropertyGroup></Project>`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, _ := findTech(result.Technologies, ".NET Full Framework")
	if tech.Version != "4.7.2" {
		t.Fatalf("expected 4.7.2, got %q", tech.Version)
	}
}

// A solution mixing an SDK-style project with classic ones has to report both,
// which is the shape that sent a Full Framework solution in as .NET Core only
func TestScan_ClassicAndSdkProjectsTogether_ReportsBoth(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "classic/Caterex.Data.csproj", `<Project><PropertyGroup><TargetFrameworkVersion>v4.8</TargetFrameworkVersion></PropertyGroup></Project>`)
	write(t, dir, "modern/trello-pull.csproj", `<Project><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := findTech(result.Technologies, ".NET Full Framework"); !ok {
		t.Fatalf("expected the classic project reported, got %+v", result.Technologies)
	}
	if _, ok := findTech(result.Technologies, ".NET Core SDK"); !ok {
		t.Fatalf("expected the SDK project reported, got %+v", result.Technologies)
	}
}

func TestScan_ClassicCsprojWithoutTheVeePrefix_IsStillRead(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "odd.csproj", `<Project><PropertyGroup><TargetFrameworkVersion>4.6.1</TargetFrameworkVersion></PropertyGroup></Project>`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	tech, _ := findTech(result.Technologies, ".NET Full Framework")
	if tech.Version != "4.6.1" {
		t.Fatalf("expected 4.6.1, got %q", tech.Version)
	}
}
