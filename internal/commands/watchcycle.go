package commands

import (
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/detect"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

// cycleResult carries what a sweep did, so the resident loop can stay quiet on
// the runs that found nothing and the watch set can be rebuilt from what the
// scan actually saw rather than from what it saw the first time.
type cycleResult struct {
	Changed  int
	Watching []string
}

// runCycle brings every linked project up to date with the machine. It is what
// the timer runs on a schedule and what the resident watcher runs when a file it
// is watching moves.
//
// It keeps what the project already tracks current, and does nothing else:
//   - a tracked technology detected at a different version is replaced
//   - a tracked dependency group whose files have moved is re-uploaded
//   - software that is not tracked is left for a person to decide at the next
//     interactive scan
//   - a technology that simply stopped being detected is left alone
//
// The last one matters most. A missing detection is far more often an unmounted
// volume or a directory this process cannot read than software that was
// uninstalled, and silently dropping a tracked technology would stop the CVE
// alerts the project exists for.
func runCycle() (cycleResult, error) {
	result := cycleResult{}

	projects, err := config.LinkedProjects()
	if err != nil {
		return result, err
	}
	if len(projects) == 0 {
		return result, nil
	}

	client, _, err := writableClient()
	if err != nil {
		return result, err
	}

	watching := map[string]bool{}
	for _, cfg := range projects {
		changed, watched, err := cycleProject(client, cfg)
		if err != nil {
			// One unreachable project must not stop the others. The error is
			// reported and the sweep carries on.
			ui.Println("  " + cfg.ProjectName + ": " + err.Error())
			continue
		}
		result.Changed += changed
		for _, path := range watched {
			watching[path] = true
		}
	}

	for path := range watching {
		result.Watching = append(result.Watching, path)
	}
	return result, nil
}

func cycleProject(client *api.Client, cfg *config.ProjectConfig) (int, []string, error) {
	project, err := client.GetProject(cfg.ProjectID)
	if err != nil {
		if isNotFound(err) {
			// Deleted on the website. Nothing to update and nothing to fix
			// here, so it is skipped rather than treated as a failure.
			return 0, nil, nil
		}
		return 0, nil, err
	}

	deps, err := reconcileTracked(client, project, cfg)
	if err != nil {
		return 0, nil, err
	}

	if len(cfg.Paths) == 0 {
		return 0, nil, nil
	}

	changed := 0
	var watched []string

	// Technologies are merged across every path and settled once, after the
	// loop. Applying them per path would let two directories of one project
	// that hold the same software at different versions take it in turns to
	// replace each other, so every sweep would delete and re-add a row forever.
	// Dependency groups are named per directory, so they have no such contest
	// and are applied as each path is read.
	merged := &detect.Result{}
	unresolved := map[string]bool{}

	// Two directories can produce the same group name, because a group is named
	// after its manifest's parent directory. Refreshing each name once a sweep
	// stops two of them overwriting each other's contents and each other's
	// digest, which would otherwise re-upload on every single sweep forever.
	refreshed := map[string]bool{}

	// Whether every path was actually read. detect.Scan reports success for a
	// root it could not open at all, because the walk swallows the error, so a
	// project on an unmounted volume comes back looking like a machine with
	// nothing installed in it. Nothing may be retired on that evidence.
	complete := true

	for _, dir := range cfg.Paths {
		if !readable(dir) {
			complete = false
			continue
		}

		scanned, err := detect.Scan(dir)
		if err != nil {
			complete = false
			continue
		}

		for name := range resolveVersionLinesReporting(client, scanned, cfg.Technologies) {
			unresolved[name] = true
		}
		watched = append(watched, watchPaths(scanned)...)
		merged.Technologies = append(merged.Technologies, scanned.Technologies...)

		applied, err := applyGroups(client, project.ID, dir, scanned, deps, refreshed, cfg)
		if err != nil {
			return changed, watched, err
		}
		changed += applied
	}

	detect.Dedupe(merged)

	applied, err := applyCycleTechnologies(client, project.ID, merged, unresolved, complete, cfg)
	changed += applied
	if err != nil {
		return changed, watched, err
	}

	if err := config.SaveProject(cfg.Paths[0], cfg); err != nil {
		return changed, watched, err
	}
	return changed, watched, nil
}

// The order here is the safety property, not a detail. The new version is added
// first and the superseded row retired afterwards, so a crash, a network drop or
// a killed service in between leaves the project tracking BOTH versions rather
// than neither. Tracking one version too many is noise that the next sweep
// clears up by itself; tracking none is a technology that has silently stopped
// being watched for CVEs, which is the failure this whole feature exists to
// prevent.
func applyCycleTechnologies(
	client *api.Client,
	projectID int,
	merged *detect.Result,
	unresolved map[string]bool,
	complete bool,
	cfg *config.ProjectConfig,
) (int, error) {
	save := func() error { return config.SaveProject(cfg.Paths[0], cfg) }
	before := len(cfg.Technologies)

	// Worked out before anything is written, while every tracked row is still
	// present, because the moves are identified by comparing the two sides.
	moves := movedVersions(merged.Technologies, unresolved, cfg)
	chosen := cycleSelection(merged, moves, cfg)

	if err := applyTechnologies(client, projectID, merged.Technologies, chosen, cfg, save); err != nil {
		return 0, err
	}
	if err := applyKernels(client, merged.Technologies, chosen, cfg, save); err != nil {
		return 0, err
	}

	added := len(cfg.Technologies) - before
	if !complete {
		// Some directory could not be read, so what was not detected proves
		// nothing about what is installed.
		return added, nil
	}

	retired, err := retireSupersededVersions(client, moves, cfg, save)
	return added + retired, err
}

func applyGroups(
	client *api.Client,
	projectID int,
	dir string,
	scanned *detect.Result,
	deps *api.DependencySummary,
	refreshed map[string]bool,
	cfg *config.ProjectConfig,
) (int, error) {
	save := func() error { return config.SaveProject(dir, cfg) }
	before := len(cfg.DependencyGrp)

	primaries := primaryManifests(scanned)
	manifests := cycleManifestSelection(dir, primaries, cfg)
	if err := applyManifests(client, projectID, dir, primaries, scanned.Manifests, manifests, cfg, save); err != nil {
		return 0, err
	}

	reuploaded, err := refreshTrackedGroups(client, projectID, dir, primaries, scanned.Manifests, deps, refreshed, cfg)
	if err != nil {
		return reuploaded, err
	}

	return reuploaded + len(cfg.DependencyGrp) - before, nil
}

// move is one technology whose single tracked row and single detection disagree
// about the version. Both sides being singular is the whole safety argument: it
// is the only shape in which "this install moved" is the sole explanation.
type move struct {
	Name       string
	FromID     int
	FromVer    string
	ToVer      string
	DetectedAt int
}

// movedVersions finds the technologies whose version has demonstrably moved.
//
// It acts only where the evidence is unambiguous: exactly one tracked row of a
// name, and exactly one detected version of it. Every other shape is left
// completely alone, because in every other shape "the install moved" is not the
// only explanation:
//
//   - two tracked rows: the project may follow two installs, or a second machine
//     or the website may own the other row. Guessing which one moved deletes
//     somebody else's.
//   - two detections: a Dockerfile naming one release while the host runs
//     another is the ordinary case, not a move, and the two are not comparable.
//   - no detection: nothing was found, which an unreadable directory and an
//     uninstall produce identically.
//   - an unresolved version line: the tracked side is a catalog line and the
//     detected side is raw, so the two cannot be compared at all.
func movedVersions(
	detected []detect.Technology,
	unresolved map[string]bool,
	cfg *config.ProjectConfig,
) []move {
	found := detectedByName(detected)
	var moves []move

	for _, name := range trackedNames(cfg) {
		if unresolved[name] {
			continue
		}
		if countTracked(cfg, name) != 1 || len(found[name]) != 1 {
			continue
		}

		at := trackedAt(cfg, name)
		row := cfg.Technologies[at]
		seen := found[name][0]

		if row.ID == 0 || row.Version == "" || row.Version == detected[seen].Version {
			continue
		}

		moves = append(moves, move{
			Name:       row.Name,
			FromID:     row.ID,
			FromVer:    row.Version,
			ToVer:      detected[seen].Version,
			DetectedAt: seen,
		})
	}
	return moves
}

// retireSupersededVersions removes the rows the moves have replaced. It runs
// after the replacements have been added, so a crash in between leaves the
// project tracking both versions rather than neither.
func retireSupersededVersions(
	client *api.Client,
	moves []move,
	cfg *config.ProjectConfig,
	save func() error,
) (int, error) {
	retired := 0

	for _, moved := range moves {
		// The add has to have landed. Retiring on the strength of an add that
		// did not happen is the one way this loses a technology outright.
		if !trackedTechKeys(cfg)[techKey(moved.Name, moved.ToVer)] {
			continue
		}

		if err := client.DeleteTechnology(moved.FromID); err != nil {
			return retired, err
		}
		removeTracked(cfg, moved.FromID)
		if err := save(); err != nil {
			return retired, err
		}

		ui.Println("  " + moved.Name + " moved " + moved.FromVer + " -> " + moved.ToVer)
		retired++
	}
	return retired, nil
}

func removeTracked(cfg *config.ProjectConfig, id int) {
	for i, t := range cfg.Technologies {
		if t.ID == id {
			cfg.Technologies = append(cfg.Technologies[:i], cfg.Technologies[i+1:]...)
			return
		}
	}
}

// detectedByName indexes detections by name, keeping their positions so the
// selection can tick the exact entry. A detection that could not read a version
// says nothing about what is installed, so it is left out entirely rather than
// counted as one more thing found.
func detectedByName(detected []detect.Technology) map[string][]int {
	out := map[string][]int{}
	for i, found := range detected {
		if found.Version == "" {
			continue
		}
		name := strings.ToLower(found.Name)
		out[name] = append(out[name], i)
	}
	return out
}

func trackedNames(cfg *config.ProjectConfig) []string {
	seen := map[string]bool{}
	var names []string
	for _, t := range cfg.Technologies {
		name := strings.ToLower(t.Name)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func countTracked(cfg *config.ProjectConfig, name string) int {
	count := 0
	for _, t := range cfg.Technologies {
		if strings.EqualFold(t.Name, name) {
			count++
		}
	}
	return count
}

func trackedAt(cfg *config.ProjectConfig, name string) int {
	for i, t := range cfg.Technologies {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

// cycleSelection ticks exactly two kinds of detection, and nothing else:
//
//   - one whose name and version are already tracked, so applyKernels can record
//     a newly booted kernel against it. applyTechnologies skips it, since it is
//     already there.
//   - the replacement half of an unambiguous move, which is the one thing this
//     service exists to add.
//
// Everything else is left for a person. Ticking by name alone was wrong: it
// added a second concurrent install nobody chose, and put back any row the user
// deleted, within one sweep, for as long as another row of that name survived.
func cycleSelection(scanned *detect.Result, moves []move, cfg *config.ProjectConfig) []ui.Item {
	replacing := map[int]bool{}
	for _, moved := range moves {
		replacing[moved.DetectedAt] = true
	}

	tracked := trackedTechKeys(cfg)
	items := make([]ui.Item, len(scanned.Technologies))
	for i, t := range scanned.Technologies {
		items[i] = ui.Item{Selected: replacing[i] || tracked[techKey(t.Name, t.Version)]}
	}
	return items
}

func cycleManifestSelection(dir string, primaries []detect.Manifest, cfg *config.ProjectConfig) []ui.Item {
	tracked := trackedGroupNames(cfg)
	items := make([]ui.Item, len(primaries))
	for i, m := range primaries {
		items[i] = ui.Item{Selected: tracked[groupNameFor(dir, m)]}
	}
	return items
}

// refreshTrackedGroups re-uploads a dependency group whose files have changed.
// A lock file is where a dependency version actually lives, so a group left at
// the contents of the first scan would report advisories against packages that
// were upgraded months ago.
func refreshTrackedGroups(
	client *api.Client,
	projectID int,
	dir string,
	primaries []detect.Manifest,
	all []detect.Manifest,
	deps *api.DependencySummary,
	refreshed map[string]bool,
	cfg *config.ProjectConfig,
) (int, error) {
	if deps == nil {
		return 0, nil
	}

	uploaded := 0
	for _, primary := range primaries {
		groupName := groupNameFor(dir, primary)
		if !trackedGroupNames(cfg)[groupName] || refreshed[groupName] {
			continue
		}
		refreshed[groupName] = true

		bundle := append([]detect.Manifest{primary}, supportingFor(primary, all)...)
		digest := manifestDigest(bundle)
		if digest == storedDigest(cfg, groupName) {
			continue
		}

		files := make([]api.ManifestFile, len(bundle))
		for i, m := range bundle {
			files[i] = api.ManifestFile{FileName: m.FileName, Content: m.Content}
		}

		if _, err := client.UploadManifests(projectID, api.UploadManifestsRequest{
			Ecosystem: primary.Ecosystem,
			GroupName: groupName,
			Files:     files,
		}); err != nil {
			return uploaded, err
		}

		setDigest(cfg, groupName, digest)
		ui.Println("  refreshed " + groupName)
		uploaded++
	}
	return uploaded, nil
}
