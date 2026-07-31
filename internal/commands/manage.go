package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

func Remove(args []string) error {
	client, _, err := authenticatedClient()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	return remove(client, dir)
}

func remove(client *api.Client, dir string) error {
	cfg, err := config.LoadProject(dir)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.ProjectID == 0 {
		return errNoProjectLink
	}

	project, err := client.GetProject(cfg.ProjectID)
	if err != nil {
		return err
	}
	deps, err := client.GetDependencies(cfg.ProjectID)
	if err != nil {
		return err
	}

	if len(project.Technologies) == 0 && len(deps.Groups) == 0 {
		ui.Println("This system has nothing to remove.")
		return nil
	}

	if err := removeTechnologies(client, dir, cfg, project.Technologies); err != nil {
		return err
	}
	return removeDependencyGroups(client, dir, cfg, deps.Groups)
}

func removeTechnologies(client *api.Client, dir string, cfg *config.ProjectConfig, techs []api.Technology) error {
	if len(techs) == 0 {
		return nil
	}

	items := make([]ui.Item, len(techs))
	for i, t := range techs {
		items[i] = ui.Item{Label: label(t.Name, t.Version), Selected: false}
	}

	chosen := ui.ToggleList("Select technologies to REMOVE:", items)

	removed, err := deleteChosenTechnologies(client, techs, chosen)
	if err != nil {
		return err
	}
	reportRemoved(len(removed), "technologies")

	cfg.Technologies = filterTech(cfg.Technologies, removed)
	return config.SaveProject(dir, cfg)
}

func deleteChosenTechnologies(client *api.Client, techs []api.Technology, chosen []ui.Item) (map[string]bool, error) {
	removed := map[string]bool{}
	for i, item := range chosen {
		if !item.Selected {
			continue
		}
		t := techs[i]
		if err := client.DeleteTechnology(t.ID); err != nil {
			return removed, err
		}
		removed[techKey(t.Name, t.Version)] = true
		ui.Println("  removed technology: " + label(t.Name, t.Version))
	}
	return removed, nil
}

func removeDependencyGroups(client *api.Client, dir string, cfg *config.ProjectConfig, groups []api.DependencyGroupInfo) error {
	if len(groups) == 0 {
		return nil
	}

	items := make([]ui.Item, len(groups))
	for i, g := range groups {
		items[i] = ui.Item{Label: g.Name + " (" + ecosystemLabel(g.Ecosystem) + ")", Selected: false}
	}

	chosen := ui.ToggleList("Select dependency groups to REMOVE:", items)

	removed, err := deleteChosenGroups(client, groups, chosen)
	if err != nil {
		return err
	}
	reportRemoved(len(removed), "dependency groups")

	cfg.DependencyGrp = filterGroups(cfg.DependencyGrp, removed)
	return config.SaveProject(dir, cfg)
}

func deleteChosenGroups(client *api.Client, groups []api.DependencyGroupInfo, chosen []ui.Item) (map[string]bool, error) {
	removed := map[string]bool{}
	for i, item := range chosen {
		if !item.Selected {
			continue
		}
		g := groups[i]
		if err := client.DeleteDependencyGroup(g.ID); err != nil {
			return removed, err
		}
		removed[g.Name] = true
		ui.Println("  removed dependency group: " + g.Name)
	}
	return removed, nil
}

func Status(args []string) error {
	client, _, err := authenticatedClient()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	return status(client, dir)
}

func status(client *api.Client, dir string) error {
	cfg, err := config.LoadProject(dir)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.ProjectID == 0 {
		return errNoProjectLink
	}

	project, err := client.GetProject(cfg.ProjectID)
	if err != nil {
		return err
	}
	deps, err := client.GetDependencies(cfg.ProjectID)
	if err != nil {
		return err
	}

	ui.Println("System: " + project.Name)
	ui.Printf("Technologies: %d\n", len(project.Technologies))
	for _, t := range project.Technologies {
		ui.Println("  - " + label(t.Name, t.Version))
	}
	vulnNote := ""
	if deps.VulnerableCount > 0 {
		vulnNote = fmt.Sprintf(", %d with known CVEs", deps.VulnerableCount)
	}
	ui.Printf("Dependencies: %d packages across %d groups%s\n", deps.TotalCount, len(deps.Groups), vulnNote)
	for _, g := range deps.Groups {
		ui.Printf("  - %s (%s): %d packages\n", g.Name, ecosystemLabel(g.Ecosystem), g.DependencyCount)
	}
	return nil
}

func filterTech(techs []config.TrackedTechnology, removed map[string]bool) []config.TrackedTechnology {
	kept := techs[:0]
	for _, t := range techs {
		if !removed[techKey(t.Name, t.Version)] {
			kept = append(kept, t)
		}
	}
	return kept
}

func filterGroups(groups []config.TrackedDependencyGroup, removed map[string]bool) []config.TrackedDependencyGroup {
	kept := groups[:0]
	for _, g := range groups {
		if !removed[g.Name] {
			kept = append(kept, g)
		}
	}
	return kept
}

// reportRemoved always says what happened. Confirming with nothing selected
// used to print nothing at all, which reads exactly like a successful removal.
func reportRemoved(count int, plural string) {
	if count == 0 {
		ui.Println("  nothing selected, no " + plural + " were removed")
		return
	}
	ui.Printf("  %d %s removed\n", count, plural)
}

func baseName(dir string) string {
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return "project"
	}
	return base
}
