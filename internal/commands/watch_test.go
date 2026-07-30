package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/detect"
)

func TestWatchPaths_Always_WatchesEveryManifestItRead(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "package-lock.json")
	write(t, lock, "{}")

	paths := watchPaths(&detect.Result{
		Manifests: []detect.Manifest{{Path: lock, FileName: "package-lock.json"}},
	})

	if !contains(paths, lock) {
		t.Fatalf("the lock file is where a dependency version lives, got %v", paths)
	}
}

func TestWatchPaths_HostSourcedTechnology_WatchesTheFileItCameFrom(t *testing.T) {
	dir := t.TempDir()
	version := filepath.Join(dir, "version.php")
	write(t, version, "<?php $wp_version='6.8';")

	paths := watchPaths(&detect.Result{
		Technologies: []detect.Technology{{Name: "WordPress", Source: detect.SourceHostPrefix + version}},
	})

	if !contains(paths, version) {
		t.Fatalf("expected the host path unwrapped and watched, got %v", paths)
	}
}

// A source that names no file at all, such as the running kernel, must not end
// up in the watch set as a path that will never exist.
func TestWatchPaths_SourceThatIsNotAFile_IsNotWatched(t *testing.T) {
	paths := watchPaths(&detect.Result{
		Technologies: []detect.Technology{{Name: "Linux", Source: detect.SourceHostKern}},
	})

	if contains(paths, detect.SourceHostKern) {
		t.Fatalf("%q is a label, not a path, got %v", detect.SourceHostKern, paths)
	}
}

func TestWatchPaths_FileThatDoesNotExist_IsNotWatched(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.json")

	paths := watchPaths(&detect.Result{
		Manifests: []detect.Manifest{{Path: missing}},
	})

	if contains(paths, missing) {
		t.Fatalf("expected a missing file left out, got %v", paths)
	}
}

func TestWatchPaths_SameFileTwice_IsWatchedOnce(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "composer.lock")
	write(t, lock, "{}")

	paths := watchPaths(&detect.Result{
		Manifests: []detect.Manifest{{Path: lock}, {Path: lock}},
	})

	seen := 0
	for _, path := range paths {
		if path == lock {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("expected one entry, got %d in %v", seen, paths)
	}
}

func TestMoved_NothingTouched_ReportsNoChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package.json")
	write(t, file, "{}")

	if moved(fingerprint([]string{file})) {
		t.Fatal("an untouched file must not wake the watcher")
	}
}

func TestMoved_ContentChanged_ReportsAChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package.json")
	write(t, file, "{}")

	before := fingerprint([]string{file})
	write(t, file, `{"name":"x"}`)

	if !moved(before) {
		t.Fatal("a rewritten manifest must wake the watcher")
	}
}

// A package manager can rewrite a lock file to the same length, so a size-only
// comparison would miss the upgrade entirely.
func TestMoved_SameSizeDifferentContent_StillReportsAChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "composer.lock")
	write(t, file, "1.2.3")

	before := fingerprint([]string{file})
	touchLater(t, file, "9.9.9")

	if !moved(before) {
		t.Fatal("a same-length rewrite must still wake the watcher")
	}
}

func TestMoved_FileDeleted_ReportsAChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package.json")
	write(t, file, "{}")

	before := fingerprint([]string{file})
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	if !moved(before) {
		t.Fatal("a deleted manifest is a stack change")
	}
}

// A file the watch set expects but which is not there yet must not report a
// change on every single sweep, or the watcher rescans forever.
func TestMoved_StillMissing_ReportsNoChange(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never.json")

	before := map[string]string{missing: stamp(missing)}

	if moved(before) {
		t.Fatal("a file that was absent and still is has not changed")
	}
}

func TestMoved_FileAppeared_ReportsAChange(t *testing.T) {
	file := filepath.Join(t.TempDir(), "appears.json")

	before := map[string]string{file: stamp(file)}
	write(t, file, "{}")

	if !moved(before) {
		t.Fatal("a manifest that has just appeared is a stack change")
	}
}

// The retry loop this closes: a sweep woken by a changed file, which then
// failed, used to leave the old stamp in place, so the same file still read as
// moved on the next tick and the cycle was retried every ten seconds for as
// long as the failure lasted.
func TestRestamp_SweepFailedWithNothingDiscovered_StillClearsTheTrigger(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package-lock.json")
	write(t, file, "1")

	before := fingerprint([]string{file})
	touchLater(t, file, "2")
	if !moved(before) {
		t.Fatal("precondition: the change must be seen")
	}

	after := restamp(before, nil)

	if moved(after) {
		t.Fatal("a failed sweep must not leave the same file reading as moved forever")
	}
}

func TestRestamp_SweepFailed_KeepsWatchingTheSameFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "composer.lock")
	write(t, file, "1")

	after := restamp(fingerprint([]string{file}), nil)

	if _, watched := after[file]; !watched {
		t.Fatalf("expected the watch set kept, got %v", after)
	}
}

func TestRestamp_SweepSucceeded_TakesTheNewWatchSet(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	fresh := filepath.Join(dir, "fresh.json")
	write(t, old, "1")
	write(t, fresh, "1")

	after := restamp(fingerprint([]string{old}), []string{fresh})

	if _, watched := after[fresh]; !watched {
		t.Fatalf("expected the newly discovered file watched, got %v", after)
	}
	if _, stale := after[old]; stale {
		t.Fatalf("a file the scan no longer reads must be dropped, got %v", after)
	}
}

func TestManifestDigest_SameContent_IsStable(t *testing.T) {
	bundle := []detect.Manifest{
		{FileName: "package.json", Content: `{"a":1}`},
		{FileName: "package-lock.json", Content: `{"b":2}`},
	}

	if manifestDigest(bundle) != manifestDigest(bundle) {
		t.Fatal("the same files must digest the same or every sweep re-uploads")
	}
}

// The walk does not promise an order, and a different order is not a change to
// the dependencies.
func TestManifestDigest_ReorderedFiles_IsTheSame(t *testing.T) {
	first := []detect.Manifest{
		{FileName: "package.json", Content: `{"a":1}`},
		{FileName: "package-lock.json", Content: `{"b":2}`},
	}
	second := []detect.Manifest{first[1], first[0]}

	if manifestDigest(first) != manifestDigest(second) {
		t.Fatal("file order must not read as a dependency change")
	}
}

func TestManifestDigest_ChangedContent_Differs(t *testing.T) {
	before := []detect.Manifest{{FileName: "composer.lock", Content: `{"v":"1.0"}`}}
	after := []detect.Manifest{{FileName: "composer.lock", Content: `{"v":"2.0"}`}}

	if manifestDigest(before) == manifestDigest(after) {
		t.Fatal("an upgraded lock file must digest differently")
	}
}

// Content moving between two files in the same group changes what is installed,
// so concatenating without a separator would hide it.
func TestManifestDigest_ContentMovedBetweenFiles_Differs(t *testing.T) {
	before := []detect.Manifest{
		{FileName: "a.json", Content: "xy"},
		{FileName: "b.json", Content: "z"},
	}
	after := []detect.Manifest{
		{FileName: "a.json", Content: "x"},
		{FileName: "b.json", Content: "yz"},
	}

	if manifestDigest(before) == manifestDigest(after) {
		t.Fatal("expected the split between files to be part of the digest")
	}
}

func TestStoredDigest_UnknownGroup_IsEmpty(t *testing.T) {
	cfg := &config.ProjectConfig{
		DependencyGrp: []config.TrackedDependencyGroup{{Name: "web npm", Digest: "abc"}},
	}

	if got := storedDigest(cfg, "api NuGet"); got != "" {
		t.Fatalf("expected nothing for an untracked group, got %q", got)
	}
}

func TestSetDigest_TrackedGroup_RecordsIt(t *testing.T) {
	cfg := &config.ProjectConfig{
		DependencyGrp: []config.TrackedDependencyGroup{{Name: "web npm"}},
	}

	setDigest(cfg, "web npm", "abc")

	if storedDigest(cfg, "web npm") != "abc" {
		t.Fatalf("expected the digest recorded, got %+v", cfg.DependencyGrp)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Writing twice inside the same filesystem timestamp granularity can leave the
// mtime unchanged, which is the case a size-only stamp would miss, so the
// second write is pushed forward deliberately.
func touchLater(t *testing.T, path, content string) {
	t.Helper()
	write(t, path, content)

	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
