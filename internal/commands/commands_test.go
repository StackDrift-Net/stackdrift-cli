package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// linkedDir returns a directory already linked to projectID, with the link
// store isolated to this test.
func linkedDir(t *testing.T, projectID int) string {
	t.Helper()
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.SaveProject(dir, &config.ProjectConfig{ProjectID: projectID, ProjectName: "Demo"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func unlinkedDir(t *testing.T) string {
	t.Helper()
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	return t.TempDir()
}

func serve(t *testing.T, handler http.HandlerFunc) *api.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return api.New(server.URL, "test-token")
}

func selected(count int, picks ...int) []ui.Item {
	items := make([]ui.Item, count)
	for _, pick := range picks {
		items[pick].Selected = true
	}
	return items
}

func TestCheck_NotLinked_StopsBeforeCallingTheServer(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	err := check(client, unlinkedDir(t))

	if !errors.Is(err, errNoProjectLink) {
		t.Fatalf("expected the not-linked guard, got %v", err)
	}
	if called {
		t.Fatal("expected no request for an unlinked directory")
	}
}

func TestCheck_NoCves_Succeeds(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"technologyCount":3,"technologyCveCount":0,"dependencyCveCount":0}`))
	})

	if err := check(client, linkedDir(t, 5)); err != nil {
		t.Fatalf("a clean project should succeed, got %v", err)
	}
}

func TestCheck_TechnologyCves_FailsWithTheCounts(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"technologyCveCount":2,"dependencyCveCount":0}`))
	})

	err := check(client, linkedDir(t, 5))

	var cveErr *CveFoundError
	if !errors.As(err, &cveErr) {
		t.Fatalf("expected a CveFoundError, got %v", err)
	}
	if cveErr.Technology != 2 || cveErr.Dependency != 0 {
		t.Fatalf("expected 2 technology CVEs, got %+v", cveErr)
	}
}

func TestCheck_DependencyCvesOnly_StillFails(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"technologyCveCount":0,"dependencyCveCount":4}`))
	})

	var cveErr *CveFoundError
	if err := check(client, linkedDir(t, 5)); !errors.As(err, &cveErr) {
		t.Fatalf("a dependency CVE alone must fail the build, got %v", err)
	}
}

func TestCheck_RequestsTheLinkedProject(t *testing.T) {
	var gotPath string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	})

	if err := check(client, linkedDir(t, 42)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/api/projects/42/stats") {
		t.Fatalf("expected the linked project id in the request, got %q", gotPath)
	}
}

func TestCheck_ServerError_IsReportedNotSwallowed(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := check(client, linkedDir(t, 5)); err == nil {
		t.Fatal("expected the server error to surface")
	}
}

func TestStatus_NotLinked_StopsBeforeCallingTheServer(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	if err := status(client, unlinkedDir(t)); !errors.Is(err, errNoProjectLink) {
		t.Fatalf("expected the not-linked guard, got %v", err)
	}
	if called {
		t.Fatal("expected no request for an unlinked directory")
	}
}

func TestStatus_ReadsBothTechnologiesAndDependencies(t *testing.T) {
	seen := map[string]bool{}
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		if strings.HasSuffix(r.URL.Path, "/dependencies") {
			_, _ = w.Write([]byte(`{"groups":[{"id":1,"name":"web npm","ecosystem":"Npm","dependencyCount":12}],"totalCount":12,"vulnerableCount":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[{"id":1,"name":"WordPress","version":"6.8.3"}]}`))
	})

	if err := status(client, linkedDir(t, 7)); err != nil {
		t.Fatal(err)
	}
	if !seen["/api/projects/7"] || !seen["/api/projects/7/dependencies"] {
		t.Fatalf("expected both endpoints read, saw %v", seen)
	}
}

func TestStatus_ProjectRequestFails_IsReported(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := status(client, linkedDir(t, 7)); err == nil {
		t.Fatal("expected the failure to surface")
	}
}

func TestRemove_NotLinked_StopsBeforeCallingTheServer(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	if err := remove(client, unlinkedDir(t)); !errors.Is(err, errNoProjectLink) {
		t.Fatalf("expected the not-linked guard, got %v", err)
	}
	if called {
		t.Fatal("expected no request for an unlinked directory")
	}
}

func TestRemove_NothingTracked_ReturnsWithoutPrompting(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dependencies") {
			_, _ = w.Write([]byte(`{"groups":[],"totalCount":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Demo","technologies":[]}`))
	})

	// An empty project must short circuit, because reaching the picker would
	// block waiting on input that a non-interactive caller cannot give.
	if err := remove(client, linkedDir(t, 7)); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteChosenTechnologies_DeletesOnlyTheSelected(t *testing.T) {
	var deleted []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, r.Method+" "+r.URL.Path)
	})

	techs := []api.Technology{
		{ID: 11, Name: "WordPress", Version: "6.8.3"},
		{ID: 12, Name: "Ubuntu", Version: "24.04"},
		{ID: 13, Name: "Laravel", Version: "11"},
	}

	removed, err := deleteChosenTechnologies(client, techs, selected(3, 1))
	if err != nil {
		t.Fatal(err)
	}

	if len(deleted) != 1 || !strings.Contains(deleted[0], "/api/technologies/12") {
		t.Fatalf("expected only the chosen technology deleted, got %v", deleted)
	}
	if !removed[techKey("Ubuntu", "24.04")] {
		t.Fatalf("expected Ubuntu recorded as removed, got %v", removed)
	}
	if removed[techKey("WordPress", "6.8.3")] {
		t.Fatal("an unselected technology must not be recorded as removed")
	}
}

func TestDeleteChosenTechnologies_NoneSelected_DeletesNothing(t *testing.T) {
	called := false
	client := serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	removed, err := deleteChosenTechnologies(client, []api.Technology{{ID: 1, Name: "A"}}, selected(1))
	if err != nil {
		t.Fatal(err)
	}
	if called || len(removed) != 0 {
		t.Fatal("expected nothing deleted when nothing is selected")
	}
}

func TestDeleteChosenTechnologies_FailureStopsAndKeepsTheRestTracked(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	techs := []api.Technology{{ID: 11, Name: "WordPress", Version: "6.8.3"}}

	removed, err := deleteChosenTechnologies(client, techs, selected(1, 0))
	if err == nil {
		t.Fatal("expected the delete failure to surface")
	}
	// A failed delete must not be recorded, or the local config would drop a
	// technology the server still has.
	if removed[techKey("WordPress", "6.8.3")] {
		t.Fatal("a failed delete must not count as removed")
	}
}

func TestDeleteChosenGroups_DeletesOnlyTheSelected(t *testing.T) {
	var deleted []string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, r.URL.Path)
	})

	groups := []api.DependencyGroupInfo{
		{ID: 3, Name: "web npm", Ecosystem: "Npm"},
		{ID: 4, Name: "ProjA", Ecosystem: "NuGet"},
	}

	removed, err := deleteChosenGroups(client, groups, selected(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || !strings.Contains(deleted[0], "/api/dependencies/groups/3") {
		t.Fatalf("expected only the chosen group deleted, got %v", deleted)
	}
	if !removed["web npm"] || removed["ProjA"] {
		t.Fatalf("expected only web npm recorded, got %v", removed)
	}
}

func decodeJSON(r *http.Request, out any) error {
	return json.NewDecoder(r.Body).Decode(out)
}

// captureOutput collects what a command prints to stdout.
//
// ui.Out is swapped as well as os.Stdout, because ui resolves its writer once
// at start-up, so replacing os.Stdout alone stops reaching anything printed
// through ui.
func captureOutput(fn func()) string {
	original := os.Stdout
	originalOut := ui.Out
	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	os.Stdout = w
	ui.Out = w
	fn()
	_ = w.Close()
	os.Stdout = original
	ui.Out = originalOut

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}
