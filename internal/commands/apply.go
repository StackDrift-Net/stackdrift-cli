package commands

import (
	"path/filepath"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/detect"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

func technologyItems(result *detect.Result, existing *config.ProjectConfig) []ui.Item {
	tracked := trackedTechKeys(existing)
	noPrior := len(tracked) == 0
	items := make([]ui.Item, len(result.Technologies))
	for i, t := range result.Technologies {
		label := t.Name
		if t.Version != "" {
			label += " " + t.Version
		}
		// Already-tracked items stay checked so a re-scan shows the previous
		// selection. Host-machine detections describe where the CLI runs, not
		// the project, so they are only offered by default on a first run.
		items[i] = ui.Item{
			Label:    label,
			Hint:     t.Source,
			Selected: tracked[techKey(t.Name, t.Version)] || (noPrior && !detect.IsHostSource(t.Source)),
		}
	}
	return items
}

func primaryManifests(result *detect.Result) []detect.Manifest {
	var primaries []detect.Manifest
	for _, m := range result.Manifests {
		if m.Primary {
			primaries = append(primaries, m)
		}
	}
	return primaries
}

func manifestItems(scanDir string, primaries, all []detect.Manifest, existing *config.ProjectConfig) []ui.Item {
	tracked := trackedGroupNames(existing)
	noPrior := len(tracked) == 0
	items := make([]ui.Item, len(primaries))
	for i, m := range primaries {
		name := groupNameFor(scanDir, m)
		// An npm manifest with no lock only carries version ranges, which is
		// weak evidence and usually a vendored copy, so it is offered rather
		// than assumed.
		items[i] = ui.Item{
			Label:    name + " (" + ecosystemLabel(m.Ecosystem) + ")",
			Hint:     manifestHint(scanDir, m, all),
			Selected: tracked[name] || (noPrior && !isUnpinnedNpm(m, all)),
		}
	}
	return items
}

func isUnpinnedNpm(primary detect.Manifest, all []detect.Manifest) bool {
	return primary.Ecosystem == "Npm" && len(supportingFor(primary, all)) == 0
}

func manifestHint(scanDir string, primary detect.Manifest, all []detect.Manifest) string {
	files := []string{manifestDisplay(scanDir, primary.Path)}
	for _, supporting := range supportingFor(primary, all) {
		files = append(files, filepath.Base(supporting.Path))
	}
	hint := strings.Join(files, " + ")
	if isUnpinnedNpm(primary, all) {
		hint += "  (no lock file, versions not pinned)"
	}
	return hint
}

func applyTechnologies(client *api.Client, projectID int, detected []detect.Technology, chosen []ui.Item, cfg *config.ProjectConfig, save func() error) error {
	tracked := trackedTechKeys(cfg)
	for i, item := range chosen {
		if !item.Selected {
			continue
		}
		t := detected[i]
		if tracked[techKey(t.Name, t.Version)] {
			continue
		}

		_, err := client.AddTechnology(projectID, api.AddTechnologyRequest{
			Name:     t.Name,
			Version:  t.Version,
			Kernel:   t.Kernel,
			Category: t.Category,
		})
		if err != nil {
			return err
		}

		cfg.Technologies = append(cfg.Technologies, config.TrackedTechnology{
			Name:     t.Name,
			Version:  t.Version,
			Kernel:   t.Kernel,
			Category: t.Category,
		})
		tracked[techKey(t.Name, t.Version)] = true
		if err := save(); err != nil {
			return err
		}
		ui.Println("  added technology: " + label(t.Name, t.Version))
	}
	return nil
}

// applyKernels records the running kernel build on a distribution entry the
// project already holds. One added by this run carried its kernel in the add
// request, so only the entries applyTechnologies skipped are left to update,
// which is the case where the distribution was added from the website.
func applyKernels(client *api.Client, detected []detect.Technology, chosen []ui.Item, cfg *config.ProjectConfig, save func() error) error {
	index := make(map[string]int, len(cfg.Technologies))
	for i, t := range cfg.Technologies {
		index[techKey(t.Name, t.Version)] = i
	}

	for i, item := range chosen {
		if !item.Selected || i >= len(detected) {
			continue
		}
		t := detected[i]
		if t.Kernel == "" {
			continue
		}

		at, ok := index[techKey(t.Name, t.Version)]
		if !ok {
			continue
		}
		// A technology added moments ago has no server id recorded yet, and
		// does not need one: its kernel went out with the add.
		if cfg.Technologies[at].ID == 0 || cfg.Technologies[at].Kernel == t.Kernel {
			continue
		}

		if err := client.SetKernel(cfg.Technologies[at].ID, t.Kernel); err != nil {
			return err
		}

		cfg.Technologies[at].Kernel = t.Kernel
		if err := save(); err != nil {
			return err
		}
		ui.Println("  set kernel on " + label(t.Name, t.Version) + ": " + t.Kernel)
	}
	return nil
}

func applyManifests(client *api.Client, projectID int, dir string, primaries, all []detect.Manifest, chosen []ui.Item, cfg *config.ProjectConfig, save func() error) error {
	tracked := trackedGroupNames(cfg)

	for i, item := range chosen {
		if !item.Selected {
			continue
		}
		primary := primaries[i]
		groupName := groupNameFor(dir, primary)
		if tracked[groupName] {
			continue
		}

		// The lock and central-version files pin the primary manifest's
		// versions, so they are always uploaded alongside it.
		bundle := append([]detect.Manifest{primary}, supportingFor(primary, all)...)
		files := make([]api.ManifestFile, len(bundle))
		names := make([]string, len(bundle))
		for j, m := range bundle {
			files[j] = api.ManifestFile{FileName: m.FileName, Content: m.Content}
			names[j] = manifestDisplay(dir, m.Path)
		}

		resp, err := client.UploadManifests(projectID, api.UploadManifestsRequest{
			Ecosystem: primary.Ecosystem,
			GroupName: groupName,
			Files:     files,
		})
		if err != nil {
			return err
		}

		// The server answers 200 even when a file produced nothing, so
		// reporting the upload without checking this claims success for a
		// group that was never created.
		unreadable := matchFiles(resp.UnsupportedFiles, names)
		empty := matchFiles(resp.EmptyFiles, names)

		if len(unreadable) > 0 {
			ui.Println("  WARNING " + groupName + ": the server could not read " + strings.Join(unreadable, ", "))
		}
		if len(empty) > 0 {
			ui.Println("  WARNING " + groupName + ": no " + ecosystemLabel(primary.Ecosystem) +
				" packages are declared in " + strings.Join(empty, ", "))
		}
		if len(unreadable)+len(empty) == len(names) {
			ui.Println("           nothing was tracked for this system")
			continue
		}

		cfg.DependencyGrp = append(cfg.DependencyGrp, config.TrackedDependencyGroup{
			Name:      groupName,
			Ecosystem: primary.Ecosystem,
			Manifests: names,
		})
		tracked[groupName] = true
		if err := save(); err != nil {
			return err
		}
		ui.Println("  uploaded " + groupName + ": " + strings.Join(names, ", "))
	}
	return nil
}

func supportingFor(primary detect.Manifest, all []detect.Manifest) []detect.Manifest {
	primaryDir := filepath.Dir(primary.Path)
	var out []detect.Manifest
	for _, m := range all {
		if m.Primary || m.Ecosystem != primary.Ecosystem {
			continue
		}
		if filepath.Dir(m.Path) == primaryDir {
			out = append(out, m)
			continue
		}
		// Central NuGet package versions apply to every project in the tree.
		if primary.Ecosystem == "NuGet" && strings.EqualFold(m.FileName, "Directory.Packages.props") {
			out = append(out, m)
		}
	}
	return out
}

func trackedGroupNames(cfg *config.ProjectConfig) map[string]bool {
	keys := map[string]bool{}
	if cfg == nil {
		return keys
	}
	for _, group := range cfg.DependencyGrp {
		keys[group.Name] = true
	}
	return keys
}

func groupNameFor(scanDir string, m detect.Manifest) string {
	if strings.HasSuffix(strings.ToLower(m.FileName), ".csproj") {
		return strings.TrimSuffix(m.FileName, filepath.Ext(m.FileName))
	}

	base := filepath.Base(filepath.Dir(m.Path))
	if filepath.Dir(m.Path) == scanDir || base == "." || base == string(filepath.Separator) {
		base = baseName(scanDir)
	}
	return base + " " + ecosystemLabel(m.Ecosystem)
}

func trackedTechKeys(cfg *config.ProjectConfig) map[string]bool {
	keys := map[string]bool{}
	if cfg == nil {
		return keys
	}
	for _, t := range cfg.Technologies {
		keys[techKey(t.Name, t.Version)] = true
	}
	return keys
}

func techKey(name, version string) string {
	return strings.ToLower(name) + "|" + version
}

func label(name, version string) string {
	if version == "" {
		return name
	}
	return name + " " + version
}

func ecosystemLabel(ecosystem string) string {
	if ecosystem == "Npm" {
		return "npm"
	}
	return ecosystem
}

// matchFiles reports which of the uploaded files the server named in one of its
// reject lists, matched by base name since the server echoes what it was sent.
func matchFiles(reported, sent []string) []string {
	if len(reported) == 0 {
		return nil
	}

	var matched []string
	for _, name := range sent {
		for _, candidate := range reported {
			if filepath.Base(candidate) == filepath.Base(name) {
				matched = append(matched, filepath.Base(name))
				break
			}
		}
	}
	return matched
}
