package commands

import (
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

// hiddenNames is the set of catalog entries this CLI must not offer, keyed the
// same way every other name comparison here is: lowercased.
//
// The catalog decides, not the CLI. The mainline Linux kernel is the entry that
// forced this: a machine running a distribution is on the distribution's kernel,
// which the CLI already records against the distribution itself, so offering the
// mainline entry as well tracked one kernel twice, the second time against a
// release line and a CVE feed that do not apply to it. Compiling that rule in
// here would need a CLI release to change it and would drift from the website
// the moment the catalog moved.
type hiddenNames map[string]bool

// loadHiddenNames asks once, for a whole scan or a whole sweep. A failure hides
// nothing: the answer is a refinement of what to offer, and a network blip must
// not silently narrow what a scan finds. That is also exactly what every CLI
// released before this endpoint existed does.
func loadHiddenNames(client *api.Client) hiddenNames {
	names, err := client.CliHiddenTechnologies()
	if err != nil {
		return hiddenNames{}
	}

	hidden := make(hiddenNames, len(names))
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key != "" {
			hidden[key] = true
		}
	}
	return hidden
}

// drop removes the hidden detections from a scan result. Only the standalone
// row goes; anything a kept entry carries, such as the running kernel build on
// a distribution, is untouched.
func (h hiddenNames) drop(result *detect.Result) {
	if len(h) == 0 || result == nil {
		return
	}

	kept := result.Technologies[:0]
	for _, t := range result.Technologies {
		if h[strings.ToLower(strings.TrimSpace(t.Name))] {
			continue
		}
		kept = append(kept, t)
	}
	result.Technologies = kept
}
