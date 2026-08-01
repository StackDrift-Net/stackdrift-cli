package commands

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func techNames(techs []config.TrackedTechnology) []string {
	names := make([]string, len(techs))
	for i, t := range techs {
		names[i] = t.Name
	}
	return names
}

func TestFilterTech_DropsOnlyTheRemovedEntries(t *testing.T) {
	tracked := []config.TrackedTechnology{
		{Name: "Laravel", Version: "11"},
		{Name: "Ubuntu", Version: "24.04"},
		{Name: "WordPress", Version: "6.8.3"},
	}
	removed := map[string]bool{techKey("Ubuntu", "24.04"): true}

	kept := filterTech(tracked, removed)

	if len(kept) != 2 {
		t.Fatalf("expected 2 kept, got %v", techNames(kept))
	}
	if kept[0].Name != "Laravel" || kept[1].Name != "WordPress" {
		t.Fatalf("expected the surrounding entries to survive in order, got %v", techNames(kept))
	}
}

func TestFilterTech_SameNameDifferentVersions_RemovesOnlyTheMatch(t *testing.T) {
	// Two WordPress installs at different versions are two separate rows, so
	// removing one must not take the other with it.
	tracked := []config.TrackedTechnology{
		{Name: "WordPress", Version: "6.8.3"},
		{Name: "WordPress", Version: "5.4.1"},
	}
	removed := map[string]bool{techKey("WordPress", "5.4.1"): true}

	kept := filterTech(tracked, removed)

	if len(kept) != 1 || kept[0].Version != "6.8.3" {
		t.Fatalf("expected only 5.4.1 removed, got %+v", kept)
	}
}

func TestFilterTech_NothingRemoved_KeepsAll(t *testing.T) {
	tracked := []config.TrackedTechnology{{Name: "Laravel", Version: "11"}}

	if kept := filterTech(tracked, map[string]bool{}); len(kept) != 1 {
		t.Fatalf("expected everything kept, got %v", techNames(kept))
	}
}

func TestFilterTech_EmptyInput_ReturnsEmpty(t *testing.T) {
	if kept := filterTech(nil, map[string]bool{"x": true}); len(kept) != 0 {
		t.Fatalf("expected empty, got %v", techNames(kept))
	}
}

func TestFilterGroups_DropsOnlyTheRemovedGroup(t *testing.T) {
	groups := []config.TrackedDependencyGroup{
		{Name: "web npm"},
		{Name: "ProjA"},
	}

	kept := filterGroups(groups, map[string]bool{"web npm": true})

	if len(kept) != 1 || kept[0].Name != "ProjA" {
		t.Fatalf("expected only ProjA kept, got %+v", kept)
	}
}

func TestTechKey_IgnoresNameCaseButNotVersion(t *testing.T) {
	if techKey("WordPress", "6.8.3") != techKey("wordpress", "6.8.3") {
		t.Fatal("technology names should match regardless of case")
	}
	if techKey("WordPress", "6.8.3") == techKey("WordPress", "6.8.4") {
		t.Fatal("different versions are different entries")
	}
}

func TestBaseName_UsesTheDirectoryName(t *testing.T) {
	if got := baseName("/srv/www/mysite"); got != "mysite" {
		t.Fatalf("expected mysite, got %q", got)
	}
}

func TestBaseName_UnnameableDirectories_FallBackToProject(t *testing.T) {
	for _, dir := range []string{".", "/", "  "} {
		if got := baseName(dir); got != "project" {
			t.Fatalf("expected the fallback for %q, got %q", dir, got)
		}
	}
}

func TestLabel_OmitsAnEmptyVersion(t *testing.T) {
	if got := label("Windows", ""); got != "Windows" {
		t.Fatalf("expected no trailing space, got %q", got)
	}
	if got := label("WordPress", "6.8.3"); got != "WordPress 6.8.3" {
		t.Fatalf("expected name and version, got %q", got)
	}
}

func TestEcosystemLabel_LowercasesNpmOnly(t *testing.T) {
	if got := ecosystemLabel("Npm"); got != "npm" {
		t.Fatalf("expected npm, got %q", got)
	}
	if got := ecosystemLabel("NuGet"); got != "NuGet" {
		t.Fatalf("expected NuGet untouched, got %q", got)
	}
}

func TestTrackedTechKeys_NilConfig_IsEmptyNotNil(t *testing.T) {
	keys := trackedTechKeys(nil)
	if keys == nil {
		t.Fatal("expected a usable map for a first run")
	}
	if len(keys) != 0 {
		t.Fatalf("expected no tracked keys, got %v", keys)
	}
}

func TestTrackedGroupNames_NilConfig_IsEmptyNotNil(t *testing.T) {
	names := trackedGroupNames(nil)
	if names == nil {
		t.Fatal("expected a usable map for a first run")
	}
	if len(names) != 0 {
		t.Fatalf("expected no tracked groups, got %v", names)
	}
}

func TestReportRemoved_NothingSelected_SaysSo(t *testing.T) {
	out := captureOutput(func() { reportRemoved(0, "technologies") })

	if !strings.Contains(out, "nothing selected") {
		t.Fatalf("confirming with nothing selected must not look like success, got %q", out)
	}
}

func TestReportRemoved_SomethingRemoved_ReportsTheCount(t *testing.T) {
	out := captureOutput(func() { reportRemoved(3, "dependency groups") })

	if !strings.Contains(out, "3 dependency groups removed") {
		t.Fatalf("expected the count reported, got %q", out)
	}
}
