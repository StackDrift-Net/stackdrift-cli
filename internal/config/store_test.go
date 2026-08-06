package config

import (
	"os"
	"path/filepath"
	"testing"
)

// What a release before v0.1.5 wrote into the directory it scanned. Scanning a
// home directory back then put it exactly where the store directory now goes.
const strandedLink = `{"version":1,"projectId":41,"projectName":"Home","technologies":[{"name":"WordPress","version":"6.8.3","category":"Framework"}],"dependencyGroups":[]}`

func strandedStore(t *testing.T, body string) (home string, store string) {
	t.Helper()

	home = t.TempDir()
	store = filepath.Join(home, ProjectFileName)
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STACKDRIFT_HOME", store)
	return home, store
}

func TestLoadProject_StorePathHoldsAnOldLink_IsReclaimedIntoTheStore(t *testing.T) {
	home, store := strandedStore(t, strandedLink)

	loaded, err := LoadProject(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.ProjectID != 41 {
		t.Fatalf("expected the stranded link to be read, got %+v", loaded)
	}
	if !loaded.Migrated {
		t.Fatal("expected the move to be reported to the caller")
	}

	info, err := os.Stat(store)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected the store path to be reclaimed as a directory")
	}

	again, err := LoadProject(home)
	if err != nil {
		t.Fatal(err)
	}
	if again == nil || again.ProjectID != 41 {
		t.Fatalf("expected the reclaimed link to resolve, got %+v", again)
	}
	if len(again.Technologies) != 1 || again.Technologies[0].Name != "WordPress" {
		t.Fatalf("expected tracked technologies to survive, got %+v", again.Technologies)
	}
	if again.Migrated {
		t.Fatal("expected the second load to be a plain read")
	}
}

func TestLoadProject_StorePathHoldsAnOldLink_UnrelatedDirectoryStillLoads(t *testing.T) {
	strandedStore(t, strandedLink)

	// The reported failure: a scan of a directory that has nothing to do with
	// the stranded link died reading the store before it could ask anything.
	loaded, err := LoadProject(t.TempDir())
	if err != nil {
		t.Fatalf("expected an unlinked directory to load cleanly, got %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected no link for an unscanned directory, got %+v", loaded)
	}
}

func TestSaveProject_StorePathHoldsAnOldLink_KeepsBothLinks(t *testing.T) {
	home, _ := strandedStore(t, strandedLink)

	dir := t.TempDir()
	if err := SaveProject(dir, &ProjectConfig{ProjectID: 61, ProjectName: "New"}); err != nil {
		t.Fatal(err)
	}

	saved, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.ProjectID != 61 {
		t.Fatalf("expected the new link to resolve, got %+v", saved)
	}

	reclaimed, err := LoadProject(home)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed == nil || reclaimed.ProjectID != 41 {
		t.Fatalf("expected the stranded link to survive the save, got %+v", reclaimed)
	}
}

func TestLinkedProjects_StorePathHoldsAnOldLink_ReturnsIt(t *testing.T) {
	strandedStore(t, strandedLink)

	projects, err := LinkedProjects()
	if err != nil {
		t.Fatalf("expected the watch service to read a stranded store, got %v", err)
	}
	if len(projects) != 1 || projects[0].ProjectID != 41 {
		t.Fatalf("expected the reclaimed link, got %+v", projects)
	}
}

func TestLoadProject_StorePathHoldsSomethingElse_KeepsTheFileAside(t *testing.T) {
	_, store := strandedStore(t, "not a link at all")

	if _, err := LoadProject(t.TempDir()); err != nil {
		t.Fatalf("expected the store path to be cleared, got %v", err)
	}

	kept, err := os.ReadFile(store + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "not a link at all" {
		t.Fatalf("expected the file to be preserved byte for byte, got %q", kept)
	}
}

func TestLoadProject_StorePathHoldsSomethingElseTwice_KeepsBothFiles(t *testing.T) {
	_, store := strandedStore(t, "first")

	if _, err := LoadProject(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(store); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProject(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	first, err := os.ReadFile(store + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "first" {
		t.Fatalf("expected the earlier backup untouched, got %q", first)
	}
	second, err := os.ReadFile(store + ".bak.2")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "second" {
		t.Fatalf("expected the later file kept under its own name, got %q", second)
	}
}

func TestLoadProject_ScanningTheDirectoryTheStoreLivesIn_IsNotALegacyLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STACKDRIFT_HOME", filepath.Join(home, ProjectFileName))

	// A link for any other directory puts a real store directory at
	// ~/.stackdrift, which the legacy lookup would otherwise try to read as a
	// link file the moment the home directory itself is scanned.
	if err := SaveProject(t.TempDir(), &ProjectConfig{ProjectID: 51}); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProject(home)
	if err != nil {
		t.Fatalf("expected scanning the store's own directory to load cleanly, got %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected no link for an unscanned directory, got %+v", loaded)
	}
}

// A link is a file. Somebody else's store is a directory of the same name, and
// scanning their home directory must not be read as finding a link there.
//
// Reported from the field: root ran a scan in /home/ubuntu, so the store
// resolved to /root/.stackdrift while the legacy lookup landed on
// /home/ubuntu/.stackdrift, which is the ubuntu user's own store directory. The
// scan died with "read /home/ubuntu/.stackdrift: is a directory".
func TestLoadProject_ScannedDirectoryHoldsAnotherStore_IsNotReadAsALink(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	scanned := t.TempDir()
	if err := os.MkdirAll(filepath.Join(scanned, ProjectFileName, "12"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProject(scanned)
	if err != nil {
		t.Fatalf("a directory of that name is not a link, got %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected no link, got %+v", cfg)
	}
}

// The same shape one level down, so a damaged store cannot take the scan with
// it either.
func TestLoadProject_StoredLinkPathIsADirectory_IsSkipped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STACKDRIFT_HOME", home)

	if err := os.MkdirAll(filepath.Join(home, "12", ProjectFileName), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProject(t.TempDir())
	if err != nil {
		t.Fatalf("expected the broken entry skipped, got %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected no link, got %+v", cfg)
	}
}
