package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

type kernelCall struct {
	path   string
	kernel string
}

// kernelRecorder answers the kernel endpoint and records what it was sent.
func kernelRecorder(t *testing.T, calls *[]kernelCall) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Kernel string `json:"kernel"`
		}
		_ = json.Unmarshal(body, &req)
		*calls = append(*calls, kernelCall{path: r.URL.Path, kernel: req.Kernel})
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestApplyKernels_TrackedDistribution_SendsTheRunningBuild(t *testing.T) {
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{ID: 54, Name: "Ubuntu", Version: "24.04"},
	}}

	if err := applyKernels(client, detected, selected(1, 0), cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected one kernel call, got %d", len(calls))
	}
	if calls[0].path != "/api/technologies/54/kernel" {
		t.Fatalf("expected the tracked technology id in the path, got %q", calls[0].path)
	}
	if calls[0].kernel != "6.8.0-136" {
		t.Fatalf("expected the running build, got %q", calls[0].kernel)
	}
	if cfg.Technologies[0].Kernel != "6.8.0-136" {
		t.Fatalf("expected the link to record the build, got %q", cfg.Technologies[0].Kernel)
	}
}

func TestApplyKernels_ServerAlreadyHasTheBuild_SendsNothing(t *testing.T) {
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{ID: 54, Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136"},
	}}

	if err := applyKernels(client, detected, selected(1, 0), cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Fatalf("expected no call when the build already matches, got %d", len(calls))
	}
}

func TestApplyKernels_JustAddedTechnology_SendsNothing(t *testing.T) {
	// The add request already carried the kernel, and the entry has no server
	// id yet, so a second call would have nowhere to go.
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136"},
	}}

	if err := applyKernels(client, detected, selected(1, 0), cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Fatalf("expected no call for an entry with no server id, got %d", len(calls))
	}
}

func TestApplyKernels_UncheckedDistribution_SendsNothing(t *testing.T) {
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{ID: 54, Name: "Ubuntu", Version: "24.04"},
	}}

	if err := applyKernels(client, detected, selected(1), cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Fatalf("expected no call for an unticked technology, got %d", len(calls))
	}
}

func TestApplyKernels_TechnologyWithoutAKernel_SendsNothing(t *testing.T) {
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	detected := []detect.Technology{{Name: "Laravel", Version: "11"}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{ID: 60, Name: "Laravel", Version: "11"},
	}}

	if err := applyKernels(client, detected, selected(1, 0), cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Fatalf("expected no call for a technology with no kernel, got %d", len(calls))
	}
}

func TestApplyKernels_NoTechnologiesDetected_SendsNothing(t *testing.T) {
	var calls []kernelCall
	client := serve(t, kernelRecorder(t, &calls))
	cfg := &config.ProjectConfig{}

	if err := applyKernels(client, nil, nil, cfg, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Fatalf("expected no call with nothing detected, got %d", len(calls))
	}
}

func TestApplyKernels_ServerRejects_ReturnsTheErrorAndLeavesTheLinkAlone(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	detected := []detect.Technology{{Name: "Ubuntu", Version: "24.04", Kernel: "6.8.0-136", Source: detect.SourceOsRelease}}
	cfg := &config.ProjectConfig{Technologies: []config.TrackedTechnology{
		{ID: 54, Name: "Ubuntu", Version: "24.04"},
	}}

	err := applyKernels(client, detected, selected(1, 0), cfg, func() error { return nil })

	if err == nil {
		t.Fatal("expected the rejection to surface")
	}
	if cfg.Technologies[0].Kernel != "" {
		t.Fatalf("expected the link to keep the old build, got %q", cfg.Technologies[0].Kernel)
	}
}
