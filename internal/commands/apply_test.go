package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

func TestGroupNameFor_Csproj_UsesProjectName(t *testing.T) {
	m := detect.Manifest{Ecosystem: "NuGet", FileName: "ProjA.csproj", Path: "/sln/ProjA/ProjA.csproj", Primary: true}
	if got := groupNameFor("/sln", m); got != "ProjA" {
		t.Fatalf("expected ProjA, got %q", got)
	}
}

func TestGroupNameFor_PackageJsonInSubdir_UsesDirAndEcosystem(t *testing.T) {
	m := detect.Manifest{Ecosystem: "Npm", FileName: "package.json", Path: "/repo/web/package.json", Primary: true}
	if got := groupNameFor("/repo", m); got != "web npm" {
		t.Fatalf("expected 'web npm', got %q", got)
	}
}

func TestPrimaryManifests_FiltersOutLockAndProps(t *testing.T) {
	result := &detect.Result{Manifests: []detect.Manifest{
		{FileName: "package.json", Primary: true},
		{FileName: "package-lock.json", Primary: false},
		{FileName: "A.csproj", Primary: true},
		{FileName: "packages.lock.json", Primary: false},
	}}
	primaries := primaryManifests(result)
	if len(primaries) != 2 {
		t.Fatalf("expected 2 primaries, got %d", len(primaries))
	}
}

func TestManifestItems_TrackedChecked_UntrackedUncheckedOnRescan(t *testing.T) {
	primaries := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/a/package.json", Primary: true},
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/b/package.json", Primary: true},
	}
	existing := &config.ProjectConfig{
		DependencyGrp: []config.TrackedDependencyGroup{{Name: "a npm"}},
	}

	items := manifestItems("/app", primaries, primaries, existing)

	if !items[0].Selected {
		t.Fatal("previously tracked group should be checked")
	}
	if items[1].Selected {
		t.Fatal("untracked group on a re-scan should be unchecked")
	}
}

func TestManifestItems_FirstRun_DefaultsChecked(t *testing.T) {
	all := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/a/package.json", Primary: true},
		{Ecosystem: "Npm", FileName: "package-lock.json", Path: "/app/a/package-lock.json"},
	}

	items := manifestItems("/app", all[:1], all, nil)

	if !items[0].Selected {
		t.Fatal("a pinned manifest should default to checked on a first run")
	}
}

func TestManifestItems_FirstRunNpmWithoutLock_IsOfferedNotAssumed(t *testing.T) {
	// A lockless package.json carries only version ranges, and is usually a
	// vendored copy, so it must not be uploaded by --yes without being asked.
	primaries := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/a/package.json", Primary: true},
	}

	items := manifestItems("/app", primaries, primaries, nil)

	if items[0].Selected {
		t.Fatal("an unpinned npm manifest should start unchecked")
	}
}

func TestManifestItems_TrackedNpmWithoutLock_StaysChecked(t *testing.T) {
	primaries := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/a/package.json", Primary: true},
	}
	existing := &config.ProjectConfig{
		DependencyGrp: []config.TrackedDependencyGroup{{Name: "a npm"}},
	}

	items := manifestItems("/app", primaries, primaries, existing)

	if !items[0].Selected {
		t.Fatal("an already tracked group stays checked even without a lock")
	}
}

func TestManifestItems_FirstRunCsproj_IsCheckedWithoutALock(t *testing.T) {
	// A csproj declares its own versions, so the npm lock rule must not
	// quietly deselect every .NET project.
	primaries := []detect.Manifest{
		{Ecosystem: "NuGet", FileName: "A.csproj", Path: "/sln/A/A.csproj", Primary: true},
	}

	items := manifestItems("/sln", primaries, primaries, nil)

	if !items[0].Selected {
		t.Fatal("a csproj should default to checked without needing a lock file")
	}
}

func TestTechnologyItems_TrackedChecked_UntrackedHostUncheckedOnRescan(t *testing.T) {
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Laravel", Version: "11", Category: "Framework", Source: "composer.json"},
		{Name: "Ubuntu", Version: "24.04", Category: "OperatingSystem", Source: detect.SourceOsRelease},
	}}
	existing := &config.ProjectConfig{
		Technologies: []config.TrackedTechnology{{Name: "Laravel", Version: "11"}},
	}

	items := technologyItems(result, existing)

	if !items[0].Selected {
		t.Fatal("previously tracked technology should be checked")
	}
	if items[1].Selected {
		t.Fatal("untracked technology on a re-scan should be unchecked")
	}
}

func TestTechnologyItems_FirstRun_HostUncheckedProjectChecked(t *testing.T) {
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Laravel", Version: "11", Source: "composer.json"},
		{Name: "Ubuntu", Version: "24.04", Source: detect.SourceOsRelease},
	}}

	items := technologyItems(result, nil)

	if !items[0].Selected {
		t.Fatal("first-run project technology should be checked")
	}
	if items[1].Selected {
		t.Fatal("first-run host technology should be unchecked")
	}
}

func TestManifestHint_ShowsBundledLock(t *testing.T) {
	all := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/package.json", Primary: true},
		{Ecosystem: "Npm", FileName: "yarn.lock", Path: "/app/yarn.lock", Primary: false},
	}
	if got := manifestHint("/app", all[0], all); got != "package.json + yarn.lock" {
		t.Fatalf("expected bundled lock in hint, got %q", got)
	}
}

func TestManifestHint_WarnsWhenNoLock(t *testing.T) {
	all := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/package.json", Primary: true},
	}
	if got := manifestHint("/app", all[0], all); !strings.Contains(got, "no lock file") {
		t.Fatalf("expected no-lock warning, got %q", got)
	}
}

func TestSupportingFor_PackageJson_BundlesSiblingLock(t *testing.T) {
	all := []detect.Manifest{
		{Ecosystem: "Npm", FileName: "package.json", Path: "/app/package.json", Primary: true},
		{Ecosystem: "Npm", FileName: "package-lock.json", Path: "/app/package-lock.json", Primary: false},
		{Ecosystem: "Npm", FileName: "package-lock.json", Path: "/other/package-lock.json", Primary: false},
	}
	support := supportingFor(all[0], all)
	if len(support) != 1 || support[0].Path != "/app/package-lock.json" {
		t.Fatalf("expected only the sibling lock, got %+v", support)
	}
}

func TestSupportingFor_Csproj_BundlesSiblingLockAndTreeWideProps(t *testing.T) {
	all := []detect.Manifest{
		{Ecosystem: "NuGet", FileName: "A.csproj", Path: "/sln/A/A.csproj", Primary: true},
		{Ecosystem: "NuGet", FileName: "packages.lock.json", Path: "/sln/A/packages.lock.json", Primary: false},
		{Ecosystem: "NuGet", FileName: "Directory.Packages.props", Path: "/sln/Directory.Packages.props", Primary: false},
		{Ecosystem: "NuGet", FileName: "B.csproj", Path: "/sln/B/B.csproj", Primary: true},
	}
	support := supportingFor(all[0], all)
	paths := map[string]bool{}
	for _, m := range support {
		paths[filepath.Base(m.Path)] = true
	}
	if !paths["packages.lock.json"] || !paths["Directory.Packages.props"] {
		t.Fatalf("expected lock + props bundled, got %+v", support)
	}
	if len(support) != 2 {
		t.Fatalf("expected exactly 2 supporting files, got %d", len(support))
	}
}
