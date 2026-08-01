package commands

import (
	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// reconcileTracked replaces the locally recorded state with what the project
// actually holds on the server, in both directions. Dropping entries the
// server no longer has lets them be added back; keeping entries it does have
// stops them being added a second time.
func reconcileTracked(client *api.Client, project *api.Project, cfg *config.ProjectConfig) (*api.DependencySummary, error) {
	cfg.Technologies = trackedFromServer(project.Technologies)

	deps, err := client.GetDependencies(project.ID)
	if err != nil {
		return nil, err
	}
	cfg.DependencyGrp = mergeGroups(deps.Groups, cfg.DependencyGrp)
	return deps, nil
}

// trackedFromServer rebuilds the technology list outright, since the server
// carries every field the link records and its versions are authoritative.
func trackedFromServer(technologies []api.Technology) []config.TrackedTechnology {
	tracked := make([]config.TrackedTechnology, 0, len(technologies))
	for _, t := range technologies {
		tracked = append(tracked, config.TrackedTechnology{
			ID:       t.ID,
			Name:     t.Name,
			Version:  t.Version,
			Kernel:   t.Kernel,
			Category: t.Category,
		})
	}
	return tracked
}

// mergeGroups keeps the local entry for a group the server still has, because
// only the local one remembers which files were uploaded, and adds a bare
// entry for a group created elsewhere.
func mergeGroups(server []api.DependencyGroupInfo, local []config.TrackedDependencyGroup) []config.TrackedDependencyGroup {
	known := make(map[string]config.TrackedDependencyGroup, len(local))
	for _, group := range local {
		known[group.Name] = group
	}

	merged := make([]config.TrackedDependencyGroup, 0, len(server))
	for _, group := range server {
		if existing, ok := known[group.Name]; ok {
			merged = append(merged, existing)
			continue
		}
		merged = append(merged, config.TrackedDependencyGroup{
			Name:      group.Name,
			Ecosystem: group.Ecosystem,
		})
	}
	return merged
}
