package commands

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// hostProbe records what the caller asked of the machine's own updater, which
// no test can stand up and which is absent on every platform these run on.
type hostProbe struct {
	Asked     int
	Announced int
}

func stubHostUpdates(t *testing.T, report api.ScanReportRequest) *hostProbe {
	t.Helper()
	probe := &hostProbe{}
	original := readHostUpdates
	t.Cleanup(func() { readHostUpdates = original })
	readHostUpdates = func(announce func()) api.ScanReportRequest {
		probe.Asked++
		if announce != nil {
			probe.Announced++
			announce()
		}
		return report
	}
	return probe
}

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

// reportBodies answers everything and keeps every scan report body, so what the
// machine said about itself can be read back.
func reportBodies(t *testing.T) (*api.Client, *[]string) {
	t.Helper()
	var bodies []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/scan-report") {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
	})
	return client, &bodies
}

func counted(pending, security int) api.ScanReportRequest {
	return api.ScanReportRequest{PendingUpdates: &pending, SecurityUpdates: &security}
}

// linkProject adds a system to whichever store is already in force, unlike
// linkedDir which sets up a fresh one and would strand the previous link.
func linkProject(t *testing.T, projectID int) {
	t.Helper()
	if err := config.SaveProject(t.TempDir(), &config.ProjectConfig{ProjectID: projectID, ProjectName: "Demo"}); err != nil {
		t.Fatal(err)
	}
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

	if _, _, err := cycleProject(client, hiddenNames{}, cfg, api.ScanReportRequest{}); err != nil {
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

	if _, _, err := cycleProject(client, hiddenNames{}, cfg, api.ScanReportRequest{}); err != nil {
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

	if _, _, err := cycleProject(client, hiddenNames{}, cfg, api.ScanReportRequest{}); err != nil {
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

	if err := client.ReportScan(7, api.ScanReportRequest{}); err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPost || path != scanReportPath {
		t.Fatalf("expected POST %s, got %s %s", scanReportPath, method, path)
	}
}

func TestReportScan_UpdatesWaiting_SendsBothCounts(t *testing.T) {
	client, bodies := reportBodies(t)

	if err := client.ReportScan(7, counted(12, 3)); err != nil {
		t.Fatal(err)
	}

	body := (*bodies)[0]
	if !strings.Contains(body, `"pendingUpdates":12`) || !strings.Contains(body, `"securityUpdates":3`) {
		t.Fatalf("got %s", body)
	}
}

// Nobody having been able to ask is not the same as nothing being due, and the
// website draws a card for the second and nothing for the first.
func TestReportScan_NobodyCouldAsk_SendsNullRatherThanZero(t *testing.T) {
	client, bodies := reportBodies(t)

	if err := client.ReportScan(7, api.ScanReportRequest{}); err != nil {
		t.Fatal(err)
	}

	body := (*bodies)[0]
	if !strings.Contains(body, `"pendingUpdates":null`) {
		t.Fatalf("expected an unknown count sent as null, got %s", body)
	}
}

func TestScan_WhenItFinishes_ReportsWhatTheMachineHasWaiting(t *testing.T) {
	client, bodies := reportBodies(t)
	stubHostUpdates(t, counted(12, 3))
	dir := linkedDir(t, 7)
	writeFile(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.9"}}`)

	if err := scan(client, dir, true); err != nil {
		t.Fatal(err)
	}

	if len(*bodies) != 1 || !strings.Contains((*bodies)[0], `"pendingUpdates":12`) {
		t.Fatalf("got %v", *bodies)
	}
}

// Asking starts a process, and every system linked on a machine gets the same
// answer, so a sweep over several of them must not ask several times.
func TestRunCycle_SeveralSystems_AsksTheMachineOnce(t *testing.T) {
	signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/me"):
			_, _ = w.Write([]byte(`{"authenticated":true,"email":"a@x","userId":"u1"}`))
		case strings.HasSuffix(r.URL.Path, "/cli-hidden"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/scan-report"):
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
		}
	})
	probe := stubHostUpdates(t, counted(3, 1))
	linkProject(t, 7)
	linkProject(t, 8)

	if _, err := runCycle(); err != nil {
		t.Fatal(err)
	}

	if probe.Asked != 1 {
		t.Fatalf("expected the machine asked once for the whole sweep, got %d", probe.Asked)
	}
	// Nobody is watching a background sweep, so there is nothing to reassure and
	// a line would only repeat itself in the service log.
	if probe.Announced != 0 {
		t.Fatal("expected a background sweep to ask quietly")
	}
}

// The query talks to a service that can sit there, and the scan has already
// printed its summary, so a silent pause at the end reads as a hang.
func TestScan_AskingTheMachine_SaysWhatItIsWaitingOn(t *testing.T) {
	client, _ := reportWatcher(t)
	probe := stubHostUpdates(t, counted(1, 0))
	dir := linkedDir(t, 7)
	writeFile(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.9"}}`)

	if err := scan(client, dir, true); err != nil {
		t.Fatal(err)
	}

	if probe.Announced != 1 {
		t.Fatalf("expected the wait announced once, got %d", probe.Announced)
	}
}

func TestCycleProject_WhenItCompletes_ReportsWhatTheMachineHasWaiting(t *testing.T) {
	client, bodies := reportBodies(t)
	cfg := scanned(t)
	cfg.ProjectID = 7

	if _, _, err := cycleProject(client, hiddenNames{}, cfg, counted(4, 1)); err != nil {
		t.Fatal(err)
	}

	if len(*bodies) != 1 || !strings.Contains((*bodies)[0], `"securityUpdates":1`) {
		t.Fatalf("got %v", *bodies)
	}
}
