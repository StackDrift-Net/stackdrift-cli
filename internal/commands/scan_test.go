package commands

import (
	"path/filepath"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestConfigFor_FirstRun_BuildsConfigFromTheProject(t *testing.T) {
	cfg := configFor(&api.Project{ID: 4, Name: "My Site"}, nil)

	if cfg.ProjectID != 4 || cfg.ProjectName != "My Site" {
		t.Fatalf("expected the project recorded, got %+v", cfg)
	}
	if len(cfg.Technologies) != 0 || len(cfg.DependencyGrp) != 0 {
		t.Fatalf("expected nothing tracked yet, got %+v", cfg)
	}
}

func TestConfigFor_Rescan_KeepsWhatIsAlreadyTracked(t *testing.T) {
	existing := &config.ProjectConfig{
		ProjectID:     4,
		ProjectName:   "Old Name",
		Technologies:  []config.TrackedTechnology{{Name: "WordPress", Version: "6.8.3"}},
		DependencyGrp: []config.TrackedDependencyGroup{{Name: "web npm"}},
	}

	cfg := configFor(&api.Project{ID: 4, Name: "New Name"}, existing)

	if len(cfg.Technologies) != 1 || len(cfg.DependencyGrp) != 1 {
		t.Fatalf("a re-scan must not drop tracked items, got %+v", cfg)
	}
	if cfg.ProjectName != "New Name" {
		t.Fatalf("expected the project name refreshed, got %q", cfg.ProjectName)
	}
}

func TestConfigFor_ProjectReplaced_TakesTheNewProjectID(t *testing.T) {
	// When the linked project is gone the caller clears the tracked lists and
	// picks another project, and the config must follow the new id.
	existing := &config.ProjectConfig{ProjectID: 4, ProjectName: "Deleted"}

	cfg := configFor(&api.Project{ID: 9, Name: "Replacement"}, existing)

	if cfg.ProjectID != 9 || cfg.ProjectName != "Replacement" {
		t.Fatalf("expected the new project, got %+v", cfg)
	}
}

func TestManifestDisplay_ShowsThePathRelativeToTheScan(t *testing.T) {
	got := manifestDisplay(filepath.FromSlash("/repo"), filepath.FromSlash("/repo/web/package.json"))
	if got != filepath.FromSlash("web/package.json") {
		t.Fatalf("expected a relative path, got %q", got)
	}
}

func TestManifestDisplay_ManifestAtTheScanRoot_IsJustTheFileName(t *testing.T) {
	got := manifestDisplay(filepath.FromSlash("/repo"), filepath.FromSlash("/repo/package.json"))
	if got != "package.json" {
		t.Fatalf("expected just the file name, got %q", got)
	}
}

func TestHasFlag_MatchesAnyAlias(t *testing.T) {
	if !hasFlag([]string{"--yes"}, "--yes", "-y") {
		t.Fatal("expected --yes to match")
	}
	if !hasFlag([]string{"-y"}, "--yes", "-y") {
		t.Fatal("expected -y to match")
	}
	if hasFlag([]string{"--yesterday"}, "--yes", "-y") {
		t.Fatal("expected an exact match only")
	}
	if hasFlag(nil, "--yes", "-y") {
		t.Fatal("expected no match for no arguments")
	}
}
