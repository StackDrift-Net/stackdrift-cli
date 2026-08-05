package commands

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

func Scan(args []string) error {
	assumeYes := hasFlag(args, "--yes", "-y")

	client, _, err := writableClient()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	return scan(client, dir, assumeYes)
}

func scan(client *api.Client, dir string, assumeYes bool) error {
	project, existing, err := resolveProject(client, dir, assumeYes)
	if err != nil {
		return err
	}

	ui.Println()
	ui.Println("Scanning " + dir + " ...")
	// Walking the tree can take a while on a large directory, so say what is
	// happening rather than leaving a silent pause that reads as a hang.
	result, err := detect.ScanWithProgress(dir, func() {
		ui.Println("Scanning for additional supported software ...")
	})
	if err != nil {
		return err
	}

	// Before anything is counted or shown, so an entry the catalog does not
	// offer to the CLI is never put to the user in the first place.
	loadHiddenNames(client).drop(result)

	if len(result.Technologies) == 0 && len(result.Manifests) == 0 {
		ui.Println("No supported technologies or dependency manifests were found here.")
		// Still a check that ran and came back empty, which is a different
		// thing from a machine nothing has looked at.
		reportScan(client, project.ID)
		return nil
	}

	cfg := configFor(project, existing)

	// The project can be edited on the website between scans, so what the
	// server holds decides what is tracked. Without this a technology removed
	// there stays listed locally, is shown as already tracked, and is silently
	// skipped instead of being added back.
	deps, err := reconcileTracked(client, project, cfg)
	if err != nil {
		return err
	}

	// Detection reports what the files say, which is not always the granularity
	// the catalog tracks releases at, so the line is settled before anything is
	// shown or sent. This runs after reconcile because the tracked side has to
	// be resolved with it, or a project recorded by an older CLI stops matching.
	resolveVersionLines(client, result, cfg.Technologies)

	primaries := primaryManifests(result)
	techItems := technologyItems(result, cfg)
	manifestItems := manifestItems(dir, primaries, result.Manifests, cfg)

	var chosenTechs, chosenManifests []ui.Item
	if assumeYes {
		// Accept the recommended defaults without prompting. Host-machine
		// detections stay off, so a project scan never adds the dev machine's OS.
		chosenTechs = techItems
		chosenManifests = manifestItems
	} else {
		chosenTechs = ui.ToggleList("Technologies detected:", techItems)
		chosenManifests = ui.ToggleList("Dependency groups detected:", manifestItems)
	}

	// Persist the project link before mutating, so an interrupted run still
	// remembers the project and does not re-add what already succeeded.
	if err := config.SaveProject(dir, cfg); err != nil {
		return err
	}

	saveConfig := func() error { return config.SaveProject(dir, cfg) }
	before := len(cfg.Technologies) + len(cfg.DependencyGrp)

	if err := applyTechnologies(client, project.ID, result.Technologies, chosenTechs, cfg, saveConfig); err != nil {
		return err
	}
	if err := applyKernels(client, result.Technologies, chosenTechs, cfg, saveConfig); err != nil {
		return err
	}
	if err := applyManifests(client, project.ID, dir, primaries, result.Manifests, chosenManifests, cfg, saveConfig); err != nil {
		return err
	}

	// Unticking something that is tracked removes it from the project. This is
	// skipped for --yes so an unattended run can only ever add, which keeps a
	// CI scan from deleting anything.
	removed := 0
	if !assumeYes {
		techsRemoved, err := removeUncheckedTechnologies(client, project.Technologies, result.Technologies, chosenTechs, cfg, saveConfig)
		if err != nil {
			return err
		}
		groupsRemoved, err := removeUncheckedGroups(client, deps.Groups, dir, primaries, chosenManifests, cfg, saveConfig)
		if err != nil {
			return err
		}
		removed = techsRemoved + groupsRemoved
	}

	if err := config.SaveProject(dir, cfg); err != nil {
		return err
	}

	ui.Println()
	if added := len(cfg.Technologies) + len(cfg.DependencyGrp) + removed - before; added == 0 && removed == 0 {
		ui.Println("No changes. System '" + project.Name + "' already matches this directory.")
	} else {
		ui.Printf("System '%s' updated: %d added, %d removed.\n", project.Name, added, removed)
	}
	if path, err := config.ProjectFilePath(project.ID); err == nil {
		ui.Println("Link saved to " + path)
	}

	reportScan(client, project.ID)

	// Asked after the summary rather than before it, so the scan the user came
	// for is finished and reported before anything else is put to them.
	offerWatchService(assumeYes)
	return nil
}

// reportScan dates the check on the website. It is the last thing a scan does
// and the least important one, so a server too old to know the endpoint, or a
// network that drops on the way out, is swallowed: turning work that already
// succeeded into a failure would be a far worse trade than an undated scan.
func reportScan(client *api.Client, projectID int) {
	_ = client.ReportScan(projectID)
}

func resolveProject(client *api.Client, dir string, assumeYes bool) (*api.Project, *config.ProjectConfig, error) {
	existing, err := config.LoadProject(dir)
	if err != nil {
		return nil, nil, err
	}

	if existing != nil && existing.Migrated {
		ui.Println("Moved the system link out of " + dir + " so it is not exposed by a web server.")
		ui.Println("The old " + config.ProjectFileName + " file was removed. If it is in git, commit that deletion.")
	}

	if existing != nil && existing.ProjectID != 0 {
		project, err := client.GetProject(existing.ProjectID)
		if err == nil {
			ui.Println("Using linked system '" + project.Name + "'.")
			return project, existing, nil
		}
		if !isNotFound(err) {
			return nil, nil, err
		}
		ui.Println("The linked system no longer exists. Choose another.")
		existing.Technologies = nil
		existing.DependencyGrp = nil
	}

	if assumeYes {
		return nil, nil, errors.New("this directory is not linked to a system yet, run scan once without --yes to choose one")
	}

	project, err := chooseProject(client)
	if err != nil {
		return nil, nil, err
	}
	return project, existing, nil
}

func hasFlag(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

func configFor(project *api.Project, existing *config.ProjectConfig) *config.ProjectConfig {
	cfg := existing
	if cfg == nil {
		cfg = &config.ProjectConfig{}
	}
	cfg.ProjectID = project.ID
	cfg.ProjectName = project.Name
	return cfg
}

func manifestDisplay(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return path
	}
	return rel
}
