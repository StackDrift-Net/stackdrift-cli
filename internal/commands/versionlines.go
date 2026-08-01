package commands

import (
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

// The catalog decides what a release line looks like, and the shape is not
// consistent between technologies: WordPress lines are 7.0, .NET lines are 8.0,
// and Laravel lines are 11 for current releases but 5.8 for older ones. A rule
// hardcoded here is wrong for whichever technology it was not written for, and
// sending a version that matches no line leaves the project showing something the
// catalog cannot reason about. So the line is resolved against what the server
// actually holds.
//
// Only the version is adjusted. Whether a build is known is the detector's
// business: version.php reports the exact build a site runs, while a composer
// constraint or a target framework only ever names the line.
func resolveVersionLines(client *api.Client, result *detect.Result, tracked []config.TrackedTechnology) {
	resolveVersionLinesReporting(client, result, tracked)
}

// resolveVersionLinesReporting does the same work and names the technologies
// whose catalog lookup failed. An interactive scan can ignore that: the worst a
// stale line costs is an entry offered twice, with a person looking at it. The
// watcher cannot ignore it, because an unresolved version read against a
// resolved one looks exactly like the software having moved, and it would act on
// that by itself. Anything in this set is left completely alone.
func resolveVersionLinesReporting(
	client *api.Client,
	result *detect.Result,
	tracked []config.TrackedTechnology,
) map[string]bool {
	unresolved := map[string]bool{}
	known := make(map[string][]string)

	lookup := func(name string) []string {
		versions, looked := known[name]
		if !looked {
			// A lookup failure must not fail the scan. Leaving the detected
			// version alone is the same behaviour as before this existed.
			var err error
			versions, err = client.GetVersions(name)
			if err != nil {
				unresolved[strings.ToLower(name)] = true
			}
			known[name] = versions
		}
		return versions
	}

	for i := range result.Technologies {
		tech := &result.Technologies[i]
		if tech.Version == "" {
			continue
		}

		if line := matchVersionLine(tech.Version, lookup(tech.Name)); line != "" {
			tech.Version = line
		}
	}

	// A project tracked by an older CLI holds the unresolved version, so
	// "Laravel 11.0" would no longer match the "Laravel 11" now being detected.
	// It would then read as untracked, show unticked, and adding it back would
	// create a second row for the same technology. Resolving the tracked side
	// the same way keeps the two in step. Only the key is rewritten; the
	// recorded id is untouched, so removal and kernel updates still address the
	// right server row.
	for i := range tracked {
		entry := &tracked[i]
		if entry.Version == "" {
			continue
		}

		if line := matchVersionLine(entry.Version, lookup(entry.Name)); line != "" {
			entry.Version = line
		}
	}

	// Resolution is what makes two detections identical, so the merge has to
	// happen after it rather than only inside the scan.
	detect.Dedupe(result)
	return unresolved
}

// matchVersionLine mirrors how the server matches a version to a release: an
// exact hit wins, otherwise the longest known line the detected version sits
// under. A version under no known line is left alone, because that is a genuine
// signal that the release is not one the catalog tracks any more.
func matchVersionLine(version string, known []string) string {
	best := ""
	for _, candidate := range known {
		if strings.EqualFold(candidate, version) {
			return candidate
		}
		if strings.HasPrefix(version, candidate+".") && len(candidate) > len(best) {
			best = candidate
		}
	}
	return best
}
