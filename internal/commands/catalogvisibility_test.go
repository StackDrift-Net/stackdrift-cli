package commands

import (
	"net/http"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

func kernelAndDistro() *detect.Result {
	return &detect.Result{Technologies: []detect.Technology{
		{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease},
		{Name: "Linux Kernel", Version: "6.8", Source: detect.SourceHostKern},
	}}
}

func detectedNames(result *detect.Result) []string {
	out := make([]string, 0, len(result.Technologies))
	for _, t := range result.Technologies {
		out = append(out, t.Name)
	}
	return out
}

func hasDetection(result *detect.Result, name string) bool {
	for _, t := range result.Technologies {
		if t.Name == name {
			return true
		}
	}
	return false
}

// The distribution services its own kernel and already carries the running
// build, so offering the mainline entry as well put the same kernel on the
// system twice, the second time against a support window it does not follow.
func TestHiddenNames_KernelHiddenByTheCatalog_IsNotOffered(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/technologies/cli-hidden" {
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`["Linux Kernel"]`))
	})

	result := kernelAndDistro()
	loadHiddenNames(client).drop(result)

	if hasDetection(result, "Linux Kernel") {
		t.Fatalf("the catalog hides it, so it must not be offered, got %v", detectedNames(result))
	}
	if !hasDetection(result, "Ubuntu") {
		t.Fatalf("the distribution is the entry that should be kept, got %v", detectedNames(result))
	}
}

// Dropping the row must not drop the build it was detected alongside. The
// kernel is still tracked, on the distribution, which is the whole reason the
// mainline row is redundant.
func TestHiddenNames_DroppingTheKernelRow_KeepsTheRunningBuild(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["Linux Kernel"]`))
	})

	result := kernelAndDistro()
	loadHiddenNames(client).drop(result)

	if len(result.Technologies) != 1 || result.Technologies[0].Kernel != "6.8.0-136" {
		t.Fatalf("expected the distribution to keep its kernel build, got %+v", result.Technologies)
	}
}

// A scan must not fail, or quietly offer less, because one extra call did not
// answer. Hiding nothing is exactly what the CLI did before this existed.
func TestHiddenNames_ServerRefuses_HidesNothing(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	result := kernelAndDistro()
	loadHiddenNames(client).drop(result)

	if len(result.Technologies) != 2 {
		t.Fatalf("a failed lookup must leave the detections alone, got %v", detectedNames(result))
	}
}

func TestHiddenNames_EmptyList_HidesNothing(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	result := kernelAndDistro()
	loadHiddenNames(client).drop(result)

	if len(result.Technologies) != 2 {
		t.Fatalf("nothing is hidden, so nothing should be dropped, got %v", detectedNames(result))
	}
}

// Detection and the catalog agree on names but not always on case, and every
// other name comparison in the CLI is case insensitive.
func TestHiddenNames_DifferentCase_StillMatches(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["  linux KERNEL  "]`))
	})

	result := kernelAndDistro()
	loadHiddenNames(client).drop(result)

	if hasDetection(result, "Linux Kernel") {
		t.Fatalf("expected a case insensitive match, got %v", detectedNames(result))
	}
}

func TestHiddenNames_NothingDetected_IsNotAnError(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["Linux Kernel"]`))
	})

	result := &detect.Result{}
	loadHiddenNames(client).drop(result)

	if len(result.Technologies) != 0 {
		t.Fatalf("expected nothing, got %v", detectedNames(result))
	}
}

// The set is read once and applied to every path of every project in a sweep.
// Asking again per directory would be one wasted round trip per path, on a
// timer, forever.
func TestLoadHiddenNames_IsAskedOncePerSweep(t *testing.T) {
	calls := 0
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`["Linux Kernel"]`))
	})

	hidden := loadHiddenNames(client)
	hidden.drop(kernelAndDistro())
	hidden.drop(kernelAndDistro())

	if calls != 1 {
		t.Fatalf("expected one lookup for the whole sweep, got %d", calls)
	}
}
