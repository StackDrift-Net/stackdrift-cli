package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

type CveFoundError struct {
	Technology int
	Dependency int
}

func (e *CveFoundError) Error() string {
	return fmt.Sprintf("%d technology CVEs and %d dependency CVEs found", e.Technology, e.Dependency)
}

func Check(args []string) error {
	client, _, err := authenticatedClient()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	return check(client, dir)
}

func check(client *api.Client, dir string) error {
	cfg, err := config.LoadProject(dir)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.ProjectID == 0 {
		return errNoProjectLink
	}

	stats, err := client.GetProjectStats(cfg.ProjectID)
	if err != nil {
		return err
	}

	ui.Println("System: " + cfg.ProjectName)
	ui.Printf("Technologies: %d (%d past end of life)\n", stats.TechnologyCount, stats.EndOfLifeCount)
	ui.Printf("Technology CVEs: %d\n", stats.TechnologyCveCount)
	ui.Println(dependencyLine(stats))
	for _, line := range groupLines(stats.Groups) {
		ui.Println(line)
	}
	ui.Printf("Dependency CVEs: %d\n", stats.DependencyCveCount)

	if stats.TechnologyCveCount > 0 || stats.DependencyCveCount > 0 {
		return &CveFoundError{Technology: stats.TechnologyCveCount, Dependency: stats.DependencyCveCount}
	}

	ui.Println("No known CVEs.")
	return nil
}

// An older server sends no groups at all, so the totals stay off the line
// rather than being reported as zero, which would read as "nothing is behind".
func dependencyLine(stats *api.ProjectStats) string {
	line := fmt.Sprintf("Dependencies: %d (%d vulnerable", stats.DependencyCount, stats.VulnerableDependencyCount)
	if len(stats.Groups) > 0 {
		outdated := 0
		for _, g := range stats.Groups {
			outdated += g.OutdatedCount
		}
		line += fmt.Sprintf(", %d out of date", outdated)
	}
	return line + ")"
}

// Only groups worth acting on are listed. A group that is fully current says
// nothing, which keeps a clean project's output as short as it has always
// been, and the total on the Dependencies line means the number is never
// hidden. A group nobody has managed to check yet still gets a line, because
// staying silent about it would read as a clean bill of health.
func groupLines(groups []api.ProjectGroupStat) []string {
	shown := make([]api.ProjectGroupStat, 0, len(groups))
	for _, g := range groups {
		if g.OutdatedCount > 0 || g.UnknownCount > 0 {
			shown = append(shown, g)
		}
	}
	if len(shown) == 0 {
		return nil
	}

	// Worst first, so the group needing the most work is the one read first.
	// Name breaks the tie because group names are not unique per project and
	// the order has to be stable between runs.
	sort.SliceStable(shown, func(i, j int) bool {
		if shown[i].OutdatedCount != shown[j].OutdatedCount {
			return shown[i].OutdatedCount > shown[j].OutdatedCount
		}
		if shown[i].Name != shown[j].Name {
			return shown[i].Name < shown[j].Name
		}
		return shown[i].GroupID < shown[j].GroupID
	})

	width := 0
	for _, g := range shown {
		if n := utf8.RuneCountInString(g.Name); n > width {
			width = n
		}
	}

	lines := make([]string, 0, len(shown))
	for _, g := range shown {
		var detail string
		switch {
		case g.OutdatedCount > 0 && g.UnknownCount > 0:
			detail = fmt.Sprintf("%d of %d out of date, %d not checked",
				g.OutdatedCount, g.PackageCount, g.UnknownCount)
		case g.OutdatedCount > 0:
			detail = fmt.Sprintf("%d of %d out of date", g.OutdatedCount, g.PackageCount)
		default:
			detail = fmt.Sprintf("%d of %d not checked yet", g.UnknownCount, g.PackageCount)
		}
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(g.Name))
		lines = append(lines, "  "+g.Name+pad+"  "+detail)
	}
	return lines
}
