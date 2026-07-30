package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/detect"
)

func noSave() error { return nil }

func trackedCfg(techs ...config.TrackedTechnology) *config.ProjectConfig {
	return &config.ProjectConfig{ProjectID: 5, Technologies: techs}
}

// scanned is a config the cycle can save through, which needs a path and an
// isolated link store.
func scanned(t *testing.T, techs ...config.TrackedTechnology) *config.ProjectConfig {
	t.Helper()
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	cfg := trackedCfg(techs...)
	cfg.Paths = []string{t.TempDir()}
	return cfg
}

func noUnresolved() map[string]bool { return map[string]bool{} }

// recorder answers every call and remembers the order the writes went out in,
// which is the safety property under test: the add has to precede the delete.
func recorder(t *testing.T) (*api.Client, *[]string) {
	t.Helper()
	var calls []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"id":5,"name":"Demo","technologies":[]}`))
	})
	return client, &calls
}

func moves(detected []detect.Technology, cfg *config.ProjectConfig) []move {
	return movedVersions(detected, noUnresolved(), cfg)
}

// The bug this pins: the selection used to be computed after the old row had
// already been removed, so nothing was ticked, nothing was added, and the
// technology disappeared from the project along with its CVE alerts.
func TestApplyCycleTechnologies_VersionMoved_AddsTheNewVersion(t *testing.T) {
	client, _ := recorder(t)
	cfg := scanned(t, config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})

	merged := &detect.Result{Technologies: []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}}

	if _, err := applyCycleTechnologies(client, 5, merged, noUnresolved(), true, cfg); err != nil {
		t.Fatal(err)
	}

	if !trackedTechKeys(cfg)[techKey("WordPress", "6.8.3")] {
		t.Fatalf("the new version must end up tracked, got %+v", cfg.Technologies)
	}
	if trackedTechKeys(cfg)[techKey("WordPress", "6.8.1")] {
		t.Fatalf("the superseded version must be gone, got %+v", cfg.Technologies)
	}
}

// A crash between the two writes must leave the project tracking too much
// rather than nothing. Adding first is what guarantees that direction.
func TestApplyCycleTechnologies_VersionMoved_AddsBeforeItDeletes(t *testing.T) {
	client, calls := recorder(t)
	cfg := scanned(t, config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})

	merged := &detect.Result{Technologies: []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}}

	if _, err := applyCycleTechnologies(client, 5, merged, noUnresolved(), true, cfg); err != nil {
		t.Fatal(err)
	}

	post, del := -1, -1
	for i, call := range *calls {
		if strings.HasPrefix(call, "POST") && post < 0 {
			post = i
		}
		if strings.HasPrefix(call, "DELETE") && del < 0 {
			del = i
		}
	}
	if post < 0 || del < 0 {
		t.Fatalf("expected both an add and a delete, got %v", *calls)
	}
	if post > del {
		t.Fatalf("the delete must not precede the add, got %v", *calls)
	}
}

// Found by review: a directory on an unmounted volume scans clean, because the
// walk swallows the error for its own root, so everything tracked from inside it
// looks uninstalled.
func TestApplyCycleTechnologies_APathCouldNotBeRead_RetiresNothing(t *testing.T) {
	client, calls := recorder(t)
	cfg := scanned(t, config.TrackedTechnology{ID: 12, Name: "Ubuntu", Version: "22.04"})

	merged := &detect.Result{Technologies: []detect.Technology{
		{Name: "Ubuntu", Version: "24.04", Source: detect.SourceOsRelease},
	}}

	if _, err := applyCycleTechnologies(client, 5, merged, noUnresolved(), false, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range *calls {
		if strings.HasPrefix(call, "DELETE") {
			t.Fatalf("an incomplete scan proves nothing about what is installed, got %v", *calls)
		}
	}
}

func TestReadable_MissingDirectory_IsNotReadable(t *testing.T) {
	if readable(filepath.Join(t.TempDir(), "not-mounted")) {
		t.Fatal("a directory that is not there cannot have been scanned")
	}
}

func TestReadable_EmptyDirectory_IsReadable(t *testing.T) {
	if !readable(t.TempDir()) {
		t.Fatal("an empty directory is a real answer, not a failure")
	}
}

func TestReadable_AFile_IsNotADirectoryToScan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if readable(file) {
		t.Fatal("a file is not a scannable directory")
	}
}

func TestMovedVersions_OneRowOneDetectionDiffering_IsAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})
	detected := []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}

	got := moves(detected, cfg)

	if len(got) != 1 || got[0].FromVer != "6.8.1" || got[0].ToVer != "6.8.3" {
		t.Fatalf("expected one move, got %+v", got)
	}
}

func TestMovedVersions_SameVersion_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.3"})
	detected := []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}

	if got := moves(detected, cfg); len(got) != 0 {
		t.Fatalf("nothing moved, got %+v", got)
	}
}

// Found by review: a Dockerfile naming one release while the host runs another
// is the ordinary case, not a move, and the two are not comparable.
func TestMovedVersions_TwoDetectionsOfOneName_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "Ubuntu", Version: "22.04"})
	detected := []detect.Technology{
		{Name: "Ubuntu", Version: "22.04", Source: "Dockerfile"},
		{Name: "Ubuntu", Version: "24.04", Source: detect.SourceOsRelease},
	}

	if got := moves(detected, cfg); len(got) != 0 {
		t.Fatalf("two detections cannot say which one moved, got %+v", got)
	}
}

// Found by review: a second machine or the website can own the other row, and
// guessing which one moved deletes somebody else's.
func TestMovedVersions_TwoTrackedRowsOfOneName_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(
		config.TrackedTechnology{ID: 12, Name: "Ubuntu", Version: "22.04"},
		config.TrackedTechnology{ID: 13, Name: "Ubuntu", Version: "24.04"},
	)
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04"}}

	if got := moves(detected, cfg); len(got) != 0 {
		t.Fatalf("a project tracking two rows must not have one of them guessed away, got %+v", got)
	}
}

// The rule that matters most: an unreadable directory and an uninstall look
// identical, so nothing detected means nothing changes.
func TestMovedVersions_NothingDetected_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})

	if got := moves(nil, cfg); len(got) != 0 {
		t.Fatalf("a technology that stopped being seen must be left alone, got %+v", got)
	}
}

func TestMovedVersions_DetectionWithNoVersion_IsNotEvidence(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})
	detected := []detect.Technology{{Name: "WordPress"}}

	if got := moves(detected, cfg); len(got) != 0 {
		t.Fatalf("an unknown version is not evidence of a move, got %+v", got)
	}
}

func TestMovedVersions_RowWithNoServerId_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{Name: "WordPress", Version: "6.8.1"})
	detected := []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}

	if got := moves(detected, cfg); len(got) != 0 {
		t.Fatalf("a row with no server id cannot be retired, got %+v", got)
	}
}

// Found by review: a transient failure of the versions endpoint leaves the
// detected side raw and the tracked side resolved, which used to read as a move.
func TestMovedVersions_VersionLineLookupFailed_IsNotAMove(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 77, Name: "Laravel", Version: "11"})
	detected := []detect.Technology{{Name: "Laravel", Version: "11.9"}}

	got := movedVersions(detected, map[string]bool{"laravel": true}, cfg)

	if len(got) != 0 {
		t.Fatalf("an unresolvable name must be left entirely alone, got %+v", got)
	}
}

func TestMovedVersions_NameCaseDiffers_IsStillTheSameTechnology(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "wordpress", Version: "6.8.1"})
	detected := []detect.Technology{{Name: "WordPress", Version: "6.8.3"}}

	if got := moves(detected, cfg); len(got) != 1 {
		t.Fatalf("expected the same technology matched regardless of case, got %+v", got)
	}
}

// Retiring on the strength of an add that did not happen is the one way this
// loses a technology outright.
func TestRetireSuperseded_ReplacementNeverLanded_RetiresNothing(t *testing.T) {
	client, calls := recorder(t)
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"})

	pending := []move{{Name: "WordPress", FromID: 12, FromVer: "6.8.1", ToVer: "6.8.3"}}

	count, err := retireSupersededVersions(client, pending, cfg, noSave)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(*calls) != 0 {
		t.Fatalf("nothing may be retired until its replacement is recorded, got %v", *calls)
	}
}

func TestRetireSuperseded_ReplacementLanded_RetiresTheOldRow(t *testing.T) {
	client, calls := recorder(t)
	cfg := trackedCfg(
		config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8.1"},
		config.TrackedTechnology{ID: 13, Name: "WordPress", Version: "6.8.3"},
	)

	pending := []move{{Name: "WordPress", FromID: 12, FromVer: "6.8.1", ToVer: "6.8.3"}}

	count, err := retireSupersededVersions(client, pending, cfg, noSave)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(*calls) != 1 || !strings.HasSuffix((*calls)[0], "/api/technologies/12") {
		t.Fatalf("expected only the superseded row deleted, got %v", *calls)
	}
	if len(cfg.Technologies) != 1 || cfg.Technologies[0].Version != "6.8.3" {
		t.Fatalf("expected the installed version kept, got %+v", cfg.Technologies)
	}
}

// Found by review: ticking by name alone added a second concurrent install
// nobody chose, and put back any row the user deleted, within one sweep.
func TestCycleSelection_SecondInstallNobodyChose_IsNotAdded(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 12, Name: "WordPress", Version: "6.8"})
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "WordPress", Version: "6.8", Source: "host: /srv/a"},
		{Name: "WordPress", Version: "6.7", Source: "host: /srv/b"},
	}}

	items := cycleSelection(result, moves(result.Technologies, cfg), cfg)

	if items[1].Selected {
		t.Fatal("a second install the user never chose must not be added")
	}
}

// An already-tracked exact match stays ticked so a newly booted kernel can be
// recorded against it. applyTechnologies skips it, since it is already there.
func TestCycleSelection_ExactMatchAlreadyTracked_StaysSelected(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{Name: "Ubuntu", Version: "24.04"})
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Ubuntu", Version: "24.04", Source: detect.SourceOsRelease},
	}}

	if !cycleSelection(result, moves(result.Technologies, cfg), cfg)[0].Selected {
		t.Fatal("a tracked technology stays ticked so its kernel can be updated")
	}
}

func TestCycleSelection_UntrackedDetection_IsNotSelected(t *testing.T) {
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Laravel", Version: "11", Source: "composer.json"},
	}}

	if cycleSelection(result, moves(result.Technologies, trackedCfg()), trackedCfg())[0].Selected {
		t.Fatal("the watcher keeps tracked software current, it does not choose what to track")
	}
}

func TestCycleSelection_TheReplacementHalfOfAMove_IsSelected(t *testing.T) {
	cfg := trackedCfg(config.TrackedTechnology{ID: 3, Name: "WordPress", Version: "6.8.1"})
	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "WordPress", Version: "6.8.3", Source: "host: /srv/www"},
	}}

	if !cycleSelection(result, moves(result.Technologies, cfg), cfg)[0].Selected {
		t.Fatal("the replacement version must be added while the old row is still there")
	}
}

func TestCycleManifestSelection_UntrackedGroup_IsNotSelected(t *testing.T) {
	primaries := []detect.Manifest{{Path: "/srv/site/package.json", FileName: "package.json", Ecosystem: "Npm"}}

	if cycleManifestSelection("/srv/site", primaries, trackedCfg())[0].Selected {
		t.Fatal("a group nobody chose must not be uploaded by the watcher")
	}
}

func TestRefreshTrackedGroups_ContentChanged_ReUploads(t *testing.T) {
	uploads := 0
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			uploads++
		}
		_, _ = w.Write([]byte(`{}`))
	})

	primary := detect.Manifest{
		Path: "/srv/site/package.json", FileName: "package.json",
		Ecosystem: "Npm", Content: `{"v":"2"}`, Primary: true,
	}
	cfg := &config.ProjectConfig{
		ProjectID: 5,
		DependencyGrp: []config.TrackedDependencyGroup{
			{Name: "site npm", Ecosystem: "Npm", Digest: "stale"},
		},
	}

	count, err := refreshTrackedGroups(client, 5, "/srv/site",
		[]detect.Manifest{primary}, []detect.Manifest{primary}, &api.DependencySummary{}, map[string]bool{}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 || uploads != 1 {
		t.Fatalf("a moved lock file must be re-uploaded, count=%d uploads=%d", count, uploads)
	}
	if cfg.DependencyGrp[0].Digest == "stale" {
		t.Fatal("expected the digest updated so the next sweep stays quiet")
	}
}

func TestRefreshTrackedGroups_NothingChanged_UploadsNothing(t *testing.T) {
	uploads := 0
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			uploads++
		}
		_, _ = w.Write([]byte(`{}`))
	})

	primary := detect.Manifest{
		Path: "/srv/site/package.json", FileName: "package.json",
		Ecosystem: "Npm", Content: `{"v":"2"}`, Primary: true,
	}
	digest := manifestDigest([]detect.Manifest{primary})
	cfg := &config.ProjectConfig{
		ProjectID: 5,
		DependencyGrp: []config.TrackedDependencyGroup{
			{Name: "site npm", Ecosystem: "Npm", Digest: digest},
		},
	}

	count, err := refreshTrackedGroups(client, 5, "/srv/site",
		[]detect.Manifest{primary}, []detect.Manifest{primary}, &api.DependencySummary{}, map[string]bool{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || uploads != 0 {
		t.Fatalf("an unchanged group must not be re-sent every sweep, count=%d uploads=%d", count, uploads)
	}
}

// Adding a group is the interactive scan's decision. The refresh only keeps
// current what is already tracked.
func TestRefreshTrackedGroups_UntrackedGroup_IsNotUploaded(t *testing.T) {
	uploads := 0
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		uploads++
		_, _ = w.Write([]byte(`{}`))
	})

	primary := detect.Manifest{
		Path: "/srv/site/package.json", FileName: "package.json",
		Ecosystem: "Npm", Content: `{}`, Primary: true,
	}

	count, err := refreshTrackedGroups(client, 5, "/srv/site",
		[]detect.Manifest{primary}, []detect.Manifest{primary}, &api.DependencySummary{},
		map[string]bool{}, &config.ProjectConfig{ProjectID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || uploads != 0 {
		t.Fatalf("expected nothing uploaded, count=%d uploads=%d", count, uploads)
	}
}

func TestRunCycle_NothingLinked_DoesNothing(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	result, err := runCycle()
	if err != nil {
		t.Fatalf("a machine with no linked projects is not an error, got %v", err)
	}
	if result.Changed != 0 || len(result.Watching) != 0 {
		t.Fatalf("expected an empty sweep, got %+v", result)
	}
}
