package commands

import (
	"net/http"
	"strings"
	"testing"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
)

func group(name string, packages, outdated, unknown int) api.ProjectGroupStat {
	return api.ProjectGroupStat{
		Name:          name,
		Ecosystem:     "NuGet",
		PackageCount:  packages,
		OutdatedCount: outdated,
		UnknownCount:  unknown,
	}
}

func TestGroupLines_FullyCurrentGroup_IsNotListed(t *testing.T) {
	if lines := groupLines([]api.ProjectGroupStat{group("Clean", 12, 0, 0)}); len(lines) != 0 {
		t.Fatalf("expected a current group to stay quiet, got %q", lines)
	}
}

func TestGroupLines_OutdatedGroup_ReportsTheCountAndTheTotal(t *testing.T) {
	lines := groupLines([]api.ProjectGroupStat{group("TradeCircle.Web npm", 63, 21, 0)})

	if len(lines) != 1 {
		t.Fatalf("expected one line, got %q", lines)
	}
	if !strings.Contains(lines[0], "TradeCircle.Web npm") || !strings.Contains(lines[0], "21 of 63 out of date") {
		t.Fatalf("unexpected line %q", lines[0])
	}
}

// A group nobody has managed to check yet must not be silently treated as
// clean, which is what hiding it would amount to.
func TestGroupLines_EntirelyUncheckedGroup_IsStillListed(t *testing.T) {
	lines := groupLines([]api.ProjectGroupStat{group("Fresh upload", 40, 0, 40)})

	if len(lines) != 1 {
		t.Fatalf("expected an unchecked group to be reported, got %q", lines)
	}
	if !strings.Contains(lines[0], "40 of 40 not checked yet") {
		t.Fatalf("unexpected line %q", lines[0])
	}
}

func TestGroupLines_PartiallyUnchecked_ReportsBothNumbers(t *testing.T) {
	lines := groupLines([]api.ProjectGroupStat{group("Mixed", 10, 3, 2)})

	if !strings.Contains(lines[0], "3 of 10 out of date") || !strings.Contains(lines[0], "2 not checked") {
		t.Fatalf("expected both counts, got %q", lines[0])
	}
}

func TestGroupLines_WorstGroupFirst(t *testing.T) {
	lines := groupLines([]api.ProjectGroupStat{
		group("Small", 6, 2, 0),
		group("Biggest", 63, 21, 0),
		group("Middle", 16, 6, 0),
	})

	if !strings.Contains(lines[0], "Biggest") || !strings.Contains(lines[1], "Middle") || !strings.Contains(lines[2], "Small") {
		t.Fatalf("expected worst first, got %q", lines)
	}
}

// Group names are not unique per project, so the order has to be decided by
// something that cannot tie.
func TestGroupLines_IdenticalCountsAndNames_OrderIsStable(t *testing.T) {
	groups := []api.ProjectGroupStat{
		{Name: "Same", PackageCount: 4, OutdatedCount: 1, GroupID: 9},
		{Name: "Same", PackageCount: 4, OutdatedCount: 1, GroupID: 2},
	}

	first := groupLines(groups)
	groups[0], groups[1] = groups[1], groups[0]
	second := groupLines(groups)

	if first[0] != second[0] || first[1] != second[1] {
		t.Fatalf("expected a stable order, got %q then %q", first, second)
	}
}

func TestGroupLines_NamesAreAligned(t *testing.T) {
	lines := groupLines([]api.ProjectGroupStat{
		group("Short", 10, 5, 0),
		group("AMuchLongerGroupName", 10, 4, 0),
	})

	if strings.Index(lines[0], "4 of") != strings.Index(lines[1], "5 of") {
		t.Fatalf("expected the counts to line up, got %q", lines)
	}
}

func TestGroupLines_NoGroups_PrintsNothing(t *testing.T) {
	if lines := groupLines(nil); lines != nil {
		t.Fatalf("expected nothing for no groups, got %q", lines)
	}
}

func TestDependencyLine_WithGroups_TotalsTheOutdatedCount(t *testing.T) {
	stats := &api.ProjectStats{
		DependencyCount:           139,
		VulnerableDependencyCount: 4,
		Groups: []api.ProjectGroupStat{
			group("A", 63, 21, 0),
			group("B", 16, 6, 0),
		},
	}

	if got := dependencyLine(stats); got != "Dependencies: 139 (4 vulnerable, 27 out of date)" {
		t.Fatalf("unexpected line %q", got)
	}
}

// An older server sends no groups at all. Printing "0 out of date" there would
// be an answer we do not have.
func TestDependencyLine_NoGroups_OmitsTheOutdatedTotal(t *testing.T) {
	stats := &api.ProjectStats{DependencyCount: 139, VulnerableDependencyCount: 4}

	if got := dependencyLine(stats); got != "Dependencies: 139 (4 vulnerable)" {
		t.Fatalf("unexpected line %q", got)
	}
}

func TestCheck_ServerSendsGroups_PrintsThemUnderDependencies(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"dependencyCount":71,"vulnerableDependencyCount":1,
			"groups":[{"groupId":1,"name":"TradeCircle.Web npm","ecosystem":"Npm","packageCount":63,"outdatedCount":21,"unknownCount":0},
			          {"groupId":2,"name":"TradeCircle.Tests","ecosystem":"NuGet","packageCount":8,"outdatedCount":0,"unknownCount":0}]}`))
	})

	out := captureOutput(func() { _ = check(client, linkedDir(t, 5)) })

	if !strings.Contains(out, "Dependencies: 71 (1 vulnerable, 21 out of date)") {
		t.Fatalf("expected the total on the dependencies line, got %q", out)
	}
	if !strings.Contains(out, "TradeCircle.Web npm") {
		t.Fatalf("expected the outdated group listed, got %q", out)
	}
	if strings.Contains(out, "TradeCircle.Tests") {
		t.Fatalf("expected the current group left out, got %q", out)
	}
}

// A new CLI against a server that predates the field must degrade to the old
// output rather than claiming everything is current.
func TestCheck_ServerOmitsGroups_FallsBackToTheOldOutput(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"dependencyCount":71,"vulnerableDependencyCount":1}`))
	})

	out := captureOutput(func() { _ = check(client, linkedDir(t, 5)) })

	if !strings.Contains(out, "Dependencies: 71 (1 vulnerable)") {
		t.Fatalf("expected the pre-groups line, got %q", out)
	}
	if strings.Contains(out, "out of date") {
		t.Fatalf("expected no out-of-date claim without data, got %q", out)
	}
}

func TestCheck_ServerSendsNullGroups_DoesNotPanic(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"dependencyCount":3,"groups":null}`))
	})

	out := captureOutput(func() { _ = check(client, linkedDir(t, 5)) })

	if !strings.Contains(out, "Dependencies: 3 (0 vulnerable)") {
		t.Fatalf("expected a null groups array to read as absent, got %q", out)
	}
}
