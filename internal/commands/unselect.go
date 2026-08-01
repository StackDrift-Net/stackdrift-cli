package commands

import (
	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// removeUncheckedTechnologies deletes what the user unticked. Only something
// currently tracked AND currently detected can be removed here, so a
// technology that this directory does not surface is never touched.
func removeUncheckedTechnologies(client *api.Client, serverTechs []api.Technology, detected []detect.Technology,
	chosen []ui.Item, cfg *config.ProjectConfig, save func() error) (int, error) {

	byKey := make(map[string]api.Technology, len(serverTechs))
	for _, t := range serverTechs {
		byKey[techKey(t.Name, t.Version)] = t
	}

	removed := map[string]bool{}
	for i, item := range chosen {
		if item.Selected || i >= len(detected) {
			continue
		}

		key := techKey(detected[i].Name, detected[i].Version)
		tracked, ok := byKey[key]
		if !ok {
			continue
		}

		if err := client.DeleteTechnology(tracked.ID); err != nil {
			return len(removed), err
		}
		removed[key] = true
		cfg.Technologies = filterTech(cfg.Technologies, removed)
		if err := save(); err != nil {
			return len(removed), err
		}
		ui.Println("  removed technology: " + label(tracked.Name, tracked.Version))
	}
	return len(removed), nil
}

func removeUncheckedGroups(client *api.Client, serverGroups []api.DependencyGroupInfo, scanDir string,
	primaries []detect.Manifest, chosen []ui.Item, cfg *config.ProjectConfig, save func() error) (int, error) {

	byName := make(map[string]api.DependencyGroupInfo, len(serverGroups))
	for _, g := range serverGroups {
		byName[g.Name] = g
	}

	removed := map[string]bool{}
	for i, item := range chosen {
		if item.Selected || i >= len(primaries) {
			continue
		}

		name := groupNameFor(scanDir, primaries[i])
		tracked, ok := byName[name]
		if !ok {
			continue
		}

		if err := client.DeleteDependencyGroup(tracked.ID); err != nil {
			return len(removed), err
		}
		removed[name] = true
		cfg.DependencyGrp = filterGroups(cfg.DependencyGrp, removed)
		if err := save(); err != nil {
			return len(removed), err
		}
		ui.Println("  removed dependency group: " + name)
	}
	return len(removed), nil
}
