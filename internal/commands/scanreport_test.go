package commands

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

const scanReportPath = "/api/projects/7/scan-report"

// reportWatcher answers everything and remembers whether the scan report went
// out, which is the only thing the website has to go on: the scan itself
// happens on a machine the server never sees.
func reportWatcher(t *testing.T) (*api.Client, *bool) {
	t.Helper()
	reported := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/scan-report") {
			reported = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
	})
	return client, &reported
}

func TestScan_WhenItFinishes_ReportsTheScan(t *testing.T) {
	client, reported := reportWatcher(t)
	dir := linkedDir(t, 7)
	writeFile(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.9"}}`)

	if err := scan(client, dir, true); err != nil {
		t.Fatal(err)
	}

	if !*reported {
		t.Fatal("expected the finished scan reported so the website can date it")
	}
}

// A scan that found nothing still checked the machine, and that is exactly the
// case where knowing the check ran matters.
func TestScan_NothingDetected_StillReportsTheScan(t *testing.T) {
	client, reported := reportWatcher(t)

	if err := scan(client, linkedDir(t, 7), true); err != nil {
		t.Fatal(err)
	}

	if !*reported {
		t.Fatal("expected an empty scan reported too")
	}
}

// The report is the last thing a scan does and the least important. A server
// too old to have the endpoint, or a network that drops on the way out, must
// not turn work that already succeeded into a failure.
func TestScan_ReportRefused_DoesNotFailTheScan(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/scan-report") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
	})

	dir := linkedDir(t, 7)
	writeFile(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.9"}}`)

	if err := scan(client, dir, true); err != nil {
		t.Fatalf("a refused report must not fail the scan, got %v", err)
	}
}

func TestCycleProject_WhenItCompletes_ReportsTheScan(t *testing.T) {
	client, reported := reportWatcher(t)
	cfg := scanned(t)
	cfg.ProjectID = 7

	if _, _, err := cycleProject(client, hiddenNames{}, cfg); err != nil {
		t.Fatal(err)
	}

	if !*reported {
		t.Fatal("expected an automated sweep to report the check it just did")
	}
}

// A directory that could not be read means the versions were only partly
// checked, and claiming a clean check would hide a machine whose volume has
// gone away behind a fresh looking date.
func TestCycleProject_UnreadableDirectory_ReportsNothing(t *testing.T) {
	client, reported := reportWatcher(t)
	cfg := scanned(t)
	cfg.ProjectID = 7
	cfg.Paths = []string{filepath.Join(t.TempDir(), "gone")}

	if _, _, err := cycleProject(client, hiddenNames{}, cfg); err != nil {
		t.Fatal(err)
	}

	if *reported {
		t.Fatal("expected no report when a watched directory could not be read")
	}
}

// A project with no directories recorded has nothing to check, so a sweep over
// it proves nothing about the machine.
func TestCycleProject_NoPaths_ReportsNothing(t *testing.T) {
	client, reported := reportWatcher(t)
	cfg := &config.ProjectConfig{ProjectID: 7}

	if _, _, err := cycleProject(client, hiddenNames{}, cfg); err != nil {
		t.Fatal(err)
	}

	if *reported {
		t.Fatal("expected no report for a project with nothing to scan")
	}
}

func TestReportScan_PostsToTheSystemsReportEndpoint(t *testing.T) {
	var path, method string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.ReportScan(7); err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPost || path != scanReportPath {
		t.Fatalf("expected POST %s, got %s %s", scanReportPath, method, path)
	}
}
