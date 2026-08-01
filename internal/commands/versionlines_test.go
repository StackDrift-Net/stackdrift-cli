package commands

import (
	"net/http"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

func TestMatchVersionLine(t *testing.T) {
	// Laravel is the case that forced this: its lines are major only for current
	// releases but major.minor for the old 5.x era, so no fixed rule works.
	laravel := []string{"13", "12", "11", "10", "9", "8", "7", "6", "5.8", "5.5"}
	wordpress := []string{"7.0", "6.9", "6.8"}
	dotnet := []string{"10.0", "9.0", "8.0"}
	kernel := []string{"7.1", "7.0", "6.6", "6.12", "5.15", "5.10"}

	cases := []struct {
		name     string
		version  string
		known    []string
		expected string
	}{
		{"laravel constraint resolves to the major line", "11.0", laravel, "11"},
		{"laravel pinned build resolves to the major line", "11.5.2", laravel, "11"},
		{"laravel old era keeps its major.minor line", "5.8.38", laravel, "5.8"},
		{"laravel exact line is left alone", "11", laravel, "11"},
		{"wordpress build resolves to major.minor", "7.0.2", wordpress, "7.0"},
		{"wordpress exact line is left alone", "7.0", wordpress, "7.0"},
		{"dotnet target framework is already a line", "8.0", dotnet, "8.0"},
		{"kernel line is left alone", "6.12", kernel, "6.12"},
		// A release the catalog no longer tracks must not be forced onto a
		// neighbouring line, because being on it is the point.
		{"unknown version is left alone", "5.19", kernel, ""},
		{"no known versions leaves the version alone", "7.0.2", nil, ""},
	}

	for _, c := range cases {
		if got := matchVersionLine(c.version, c.known); got != c.expected {
			t.Errorf("%s: expected %q, got %q", c.name, c.expected, got)
		}
	}
}

// The longest match wins, so a version is never attached to a shorter line that
// happens to also be a prefix.
func TestMatchVersionLinePrefersTheLongestLine(t *testing.T) {
	known := []string{"6", "6.8"}

	if got := matchVersionLine("6.8.3", known); got != "6.8" {
		t.Fatalf("expected 6.8, got %q", got)
	}
}

// A shorter line must not swallow a different one: 6.1 is not the line for 6.12.
func TestMatchVersionLineDoesNotMatchOnDigitsAlone(t *testing.T) {
	known := []string{"6.1"}

	if got := matchVersionLine("6.12.4", known); got != "" {
		t.Fatalf("6.1 is not the line for 6.12.4, got %q", got)
	}
}

// A project linked by an older CLI holds the unresolved version, so the tracked
// side has to be resolved too. Without this the technology reads as untracked,
// shows unticked on a re-scan, and adding it back creates a second row for the
// same thing, because the server has no duplicate guard.
func TestResolveVersionLinesMigratesTheTrackedSide(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["13","12","11","10","5.8"]`))
	})

	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Laravel", Version: "11.0", Source: "composer.json"},
	}}
	tracked := []config.TrackedTechnology{{ID: 5, Name: "Laravel", Version: "11.0"}}

	resolveVersionLines(client, result, tracked)

	if result.Technologies[0].Version != "11" {
		t.Fatalf("expected the detected version resolved to 11, got %q", result.Technologies[0].Version)
	}
	if tracked[0].Version != "11" {
		t.Fatalf("expected the tracked version migrated to 11, got %q", tracked[0].Version)
	}
	if tracked[0].ID != 5 {
		t.Fatalf("the server id must survive so removal still addresses the right row, got %d", tracked[0].ID)
	}
	if techKey(result.Technologies[0].Name, result.Technologies[0].Version) != techKey(tracked[0].Name, tracked[0].Version) {
		t.Fatal("detected and tracked must share a key or the technology reads as untracked")
	}
}

// Two constraints under one major become the same row only after resolution, so
// the merge has to happen after it. Two rows sharing a key would let unticking
// one delete the technology the other represents.
func TestResolveVersionLinesMergesRowsThatBecomeIdentical(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["13","12","11"]`))
	})

	result := &detect.Result{Technologies: []detect.Technology{
		{Name: "Laravel", Version: "11.0", Source: "api/composer.json"},
		{Name: "Laravel", Version: "11.31", Source: "admin/composer.json"},
	}}

	resolveVersionLines(client, result, nil)

	if len(result.Technologies) != 1 {
		t.Fatalf("expected one Laravel row after resolution, got %d: %+v", len(result.Technologies), result.Technologies)
	}
}
