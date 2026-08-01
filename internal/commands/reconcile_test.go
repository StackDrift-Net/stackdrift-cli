package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

func TestTrackedFromServer_RemovedTechnology_IsNoLongerTracked(t *testing.T) {
	// Deleted on the website, so the link must forget it and offer it again.
	tracked := trackedFromServer([]api.Technology{{ID: 1, Name: "WordPress", Version: "6.8.3"}})

	keys := map[string]bool{}
	for _, item := range tracked {
		keys[techKey(item.Name, item.Version)] = true
	}
	if keys[techKey("Laravel", "11")] {
		t.Fatal("a technology the server no longer has must not stay tracked")
	}
	if !keys[techKey("WordPress", "6.8.3")] {
		t.Fatal("a technology the server still has must stay tracked")
	}
}

func TestTrackedFromServer_AddedOnTheWebsite_BecomesTracked(t *testing.T) {
	// Added on the website, so the CLI must not add a second copy.
	tracked := trackedFromServer([]api.Technology{
		{ID: 4, Name: "Laravel", Version: "11", Category: "Framework"},
	})

	if len(tracked) != 1 || tracked[0].Name != "Laravel" {
		t.Fatalf("expected the server's technology adopted, got %+v", tracked)
	}
	if tracked[0].ID != 4 || tracked[0].Category != "Framework" {
		t.Fatalf("expected the server's fields carried over, got %+v", tracked[0])
	}
}

func TestTrackedFromServer_EmptyProject_ClearsEverything(t *testing.T) {
	if tracked := trackedFromServer(nil); len(tracked) != 0 {
		t.Fatalf("expected nothing tracked, got %+v", tracked)
	}
}

func TestMergeGroups_ServerStillHasIt_KeepsTheUploadedFileList(t *testing.T) {
	server := []api.DependencyGroupInfo{{ID: 1, Name: "web npm", Ecosystem: "Npm"}}
	local := []config.TrackedDependencyGroup{
		{Name: "web npm", Ecosystem: "Npm", Manifests: []string{"package.json", "package-lock.json"}},
	}

	merged := mergeGroups(server, local)

	if len(merged) != 1 {
		t.Fatalf("expected one group, got %+v", merged)
	}
	// Only the local record knows which files were uploaded.
	if len(merged[0].Manifests) != 2 {
		t.Fatalf("expected the uploaded file list preserved, got %+v", merged[0])
	}
}

func TestMergeGroups_RemovedOnTheWebsite_IsDropped(t *testing.T) {
	local := []config.TrackedDependencyGroup{{Name: "web npm", Ecosystem: "Npm"}}

	if merged := mergeGroups(nil, local); len(merged) != 0 {
		t.Fatalf("expected the group dropped, got %+v", merged)
	}
}

func TestMergeGroups_CreatedElsewhere_IsAdopted(t *testing.T) {
	server := []api.DependencyGroupInfo{{ID: 9, Name: "api npm", Ecosystem: "Npm"}}

	merged := mergeGroups(server, nil)

	if len(merged) != 1 || merged[0].Name != "api npm" || merged[0].Ecosystem != "Npm" {
		t.Fatalf("expected the server group adopted, got %+v", merged)
	}
}

func TestScan_TechnologyRemovedOnTheWebsite_IsAddedBack(t *testing.T) {
	var added []api.AddTechnologyRequest
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/technologies"):
			var req api.AddTechnologyRequest
			_ = decodeJSON(r, &req)
			added = append(added, req)
			_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
		case strings.HasSuffix(r.URL.Path, "/dependencies"):
			_, _ = w.Write([]byte(`{"groups":[],"totalCount":0}`))
		default:
			// The website no longer has WordPress.
			_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
		}
	})

	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wp-admin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("wp-includes", "version.php"), "<?php\n$wp_version = '6.8.3';\n")

	// The link still claims WordPress is tracked, which is the stale state.
	if err := config.SaveProject(dir, &config.ProjectConfig{
		ProjectID:    7,
		ProjectName:  "Demo",
		Technologies: []config.TrackedTechnology{{Name: "WordPress", Version: "6.8.3", Category: "Framework"}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := scan(client, dir, true); err != nil {
		t.Fatal(err)
	}

	if len(added) != 1 || added[0].Name != "WordPress" {
		t.Fatalf("expected WordPress added back after being removed on the website, got %+v", added)
	}
}

func TestScan_TechnologyStillOnTheWebsite_IsNotDuplicated(t *testing.T) {
	var added []api.AddTechnologyRequest
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/technologies"):
			var req api.AddTechnologyRequest
			_ = decodeJSON(r, &req)
			added = append(added, req)
			_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
		case strings.HasSuffix(r.URL.Path, "/dependencies"):
			_, _ = w.Write([]byte(`{"groups":[],"totalCount":0}`))
		default:
			_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[{"id":3,"name":"WordPress","version":"6.8.3","category":"Framework"}]}`))
		}
	})

	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wp-admin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("wp-includes", "version.php"), "<?php\n$wp_version = '6.8.3';\n")

	// The link knows nothing, mimicking a technology added on the website.
	if err := config.SaveProject(dir, &config.ProjectConfig{ProjectID: 7, ProjectName: "Demo"}); err != nil {
		t.Fatal(err)
	}

	if err := scan(client, dir, true); err != nil {
		t.Fatal(err)
	}

	if len(added) != 0 {
		t.Fatalf("the server already has it, expected no second copy, got %+v", added)
	}
}

func TestRemoveUncheckedTechnologies_UntickedTrackedItem_IsDeleted(t *testing.T) {
	var deleted []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, r.Method+" "+r.URL.Path)
	})

	server := []api.Technology{{ID: 47, Name: ".NET Core SDK", Version: "8.0"}}
	detected := []detect.Technology{{Name: ".NET Core SDK", Version: "8.0"}}
	cfg := &config.ProjectConfig{
		ProjectID:    2,
		Technologies: []config.TrackedTechnology{{ID: 47, Name: ".NET Core SDK", Version: "8.0"}},
	}

	count, err := removeUncheckedTechnologies(client, server, detected, selected(1), cfg, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 || len(deleted) != 1 || !strings.Contains(deleted[0], "/api/technologies/47") {
		t.Fatalf("expected the unticked technology deleted, got %v", deleted)
	}
	if len(cfg.Technologies) != 0 {
		t.Fatalf("expected it dropped from the link, got %+v", cfg.Technologies)
	}
}

func TestRemoveUncheckedTechnologies_StillTicked_IsKept(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	server := []api.Technology{{ID: 47, Name: ".NET Core SDK", Version: "8.0"}}
	detected := []detect.Technology{{Name: ".NET Core SDK", Version: "8.0"}}

	count, err := removeUncheckedTechnologies(client, server, detected, selected(1, 0), &config.ProjectConfig{}, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if called || count != 0 {
		t.Fatal("a ticked technology must not be removed")
	}
}

func TestRemoveUncheckedTechnologies_UntrackedItem_IsNotDeleted(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	// Detected but never tracked, so unticking it removes nothing.
	detected := []detect.Technology{{Name: "Linux Mint", Version: "22"}}

	count, err := removeUncheckedTechnologies(client, nil, detected, selected(1), &config.ProjectConfig{}, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if called || count != 0 {
		t.Fatal("an untracked detection must not cause a delete")
	}
}

func TestRemoveUncheckedGroups_UntickedTrackedGroup_IsDeleted(t *testing.T) {
	var deleted []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, r.URL.Path)
	})

	server := []api.DependencyGroupInfo{{ID: 11, Name: "App"}}
	primaries := []detect.Manifest{{Ecosystem: "NuGet", FileName: "App.csproj", Path: "/proj/App.csproj", Primary: true}}
	cfg := &config.ProjectConfig{DependencyGrp: []config.TrackedDependencyGroup{{Name: "App"}}}

	count, err := removeUncheckedGroups(client, server, "/proj", primaries, selected(1), cfg, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(deleted) != 1 || !strings.Contains(deleted[0], "/api/dependencies/groups/11") {
		t.Fatalf("expected the unticked group deleted, got %v", deleted)
	}
	if len(cfg.DependencyGrp) != 0 {
		t.Fatalf("expected it dropped from the link, got %+v", cfg.DependencyGrp)
	}
}

func TestScan_WithYes_NeverRemovesAnything(t *testing.T) {
	var deleted []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/dependencies") && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"groups":[],"totalCount":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[{"id":9,"name":"Laravel","version":"11","category":"Framework"}]}`))
	})

	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "no laravel here anymore")
	if err := config.SaveProject(dir, &config.ProjectConfig{ProjectID: 7, ProjectName: "Demo"}); err != nil {
		t.Fatal(err)
	}

	// An unattended run must be add-only, so CI can never delete tracked data.
	_ = scan(client, dir, true)

	if len(deleted) != 0 {
		t.Fatalf("--yes must never delete, got %v", deleted)
	}
}
