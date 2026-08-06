package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// Three options, most often first. Advisories are published on a scale of days,
// so anything shorter than daily finds the same news sooner than anyone can act
// on it, and a menu of six made a decision out of something that has one sensible
// answer. The shorter intervals are still recognised, for installs already on
// them and for anyone who passes --interval deliberately.
var intervalChoices = []string{
	config.IntervalDaily,
	config.IntervalEveryOtherDay,
	config.IntervalWeekly,
}

func Service(args []string) error {
	action := "status"
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			action = strings.ToLower(arg)
			break
		}
	}

	switch action {
	case "install":
		return serviceInstall(args)
	case "uninstall", "remove":
		return serviceUninstall()
	case "status":
		return serviceStatus()
	case "auto-update", "autoupdate":
		return serviceAutoUpdate(args)
	default:
		return fmt.Errorf("unknown service action %q, expected install, uninstall, status or auto-update", action)
	}
}

func serviceInstall(args []string) error {
	if !service.Supported() {
		return service.ErrUnsupported
	}

	interval := intervalArg(args)
	if interval == "" {
		chosen, ok := askInterval()
		if !ok {
			return errors.New("no valid choice made")
		}
		interval = chosen
	}

	return installService(interval, resolveAutoUpdate(args))
}

func installService(interval string, autoUpdate *bool) error {
	exe, err := service.Executable()
	if err != nil {
		return err
	}

	if err := applyServicePlan(exe, interval, autoUpdate); err != nil {
		return err
	}

	ui.Println()
	ui.Println("Installed the " + service.Describe() + ", running " + config.IntervalLabel(interval) + ".")
	if config.LoadWatch().AutoUpdateEnabled() && appliesToInterval(interval) {
		ui.Println("It will install a newer CLI release before each check.")
	}
	ui.Println("Stop it any time with: stackdrift service uninstall")
	return nil
}

// Remedy rides on the error so it prints after the failure, not before
func hintedError(err error) error {
	hint := service.InstallHint(err, os.Getenv("USER"))
	if len(hint) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(hint, "\n"))
}

func serviceUninstall() error {
	if !service.Supported() {
		return service.ErrUnsupported
	}

	if err := service.Uninstall(); err != nil {
		return err
	}

	// Asked stays true so removing the service is not read as never having been
	// offered one, which would put the question again after the next scan.
	if err := config.UpdateWatch(func(s *config.WatchSettings) {
		s.Asked = true
		s.Enabled = false
	}); err != nil {
		return err
	}

	ui.Println("Removed the " + service.Describe() + ". Nothing is watching for stack changes now.")
	return nil
}

func serviceStatus() error {
	if !service.Supported() {
		ui.Println("A background service is not supported on this platform.")
		return nil
	}

	state, err := service.Status()
	if err != nil {
		return err
	}

	settings := config.LoadWatch()
	interval := state.Interval
	if interval == "" {
		interval = settings.Interval
	}

	for _, line := range statusLines(state, interval, settings) {
		ui.Println(line)
	}
	return nil
}

// statusLines is kept apart from the printing so the wording can be tested,
// which is the whole point of a status command: it is read to decide whether
// something is wrong.
func statusLines(state service.State, interval string, settings *config.WatchSettings) []string {
	if !state.Installed {
		return []string{
			"No background service is installed.",
			"Install one with: stackdrift service install",
		}
	}

	lines := []string{"Background service: " + service.Describe()}
	if state.Detail != "" {
		lines = append(lines, "  unit:     "+state.Detail)
	}
	if interval != "" {
		lines = append(lines, "  interval: "+config.IntervalLabel(interval))
	}

	// Armed is the healthy state and it is idle almost all the time, so it says
	// scheduled rather than running. The other branch stays blunt: it means the
	// scheduler is not holding the service at all and no sweep will ever fire.
	if state.Running {
		lines = append(lines, "  state:    installed and scheduled")
	} else {
		lines = append(lines, "  state:    installed but not running")
	}

	// Something that replaces an executable has to be visible to whoever reads
	// this to find out what the machine is doing.
	return append(lines, autoUpdateStatusLines(settings, interval)...)
}

func intervalArg(args []string) string {
	for i, arg := range args {
		if arg == "--interval" && i+1 < len(args) {
			return normalizeInterval(args[i+1])
		}
		if rest, found := strings.CutPrefix(arg, "--interval="); found {
			return normalizeInterval(rest)
		}
	}
	return ""
}

func normalizeInterval(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "realtime", "near-realtime", "nearrealtime":
		return config.IntervalRealtime
	case "5m", "5min", "5minutes":
		return config.IntervalFiveMin
	case "hourly", "1h":
		return config.IntervalHourly
	case "twicedaily", "twice-daily", "12h":
		return config.IntervalTwiceDay
	case "daily", "1d":
		return config.IntervalDaily
	case "everyotherday", "every-other-day", "2d":
		return config.IntervalEveryOtherDay
	case "weekly", "1w":
		return config.IntervalWeekly
	default:
		return ""
	}
}

func askInterval() (string, bool) {
	ui.Println()
	ui.Println("How often should it check?")
	for i, interval := range intervalChoices {
		ui.Printf("  %d. %s%s\n", i+1, intervalTitle(interval), intervalNote(interval))
	}
	ui.Println()

	choice, ok := ui.AskInt(fmt.Sprintf("Choose 1-%d: ", len(intervalChoices)), 1, len(intervalChoices))
	if !ok {
		return "", false
	}
	return intervalChoices[choice-1], true
}

func intervalTitle(interval string) string {
	switch interval {
	case config.IntervalRealtime:
		return "Near realtime"
	case config.IntervalFiveMin:
		return "Every 5 minutes"
	case config.IntervalHourly:
		return "Hourly"
	case config.IntervalTwiceDay:
		return "Twice a day"
	case config.IntervalDaily:
		return "Daily"
	case config.IntervalEveryOtherDay:
		return "Every other day"
	case config.IntervalWeekly:
		return "Weekly"
	default:
		return interval
	}
}

// The note is only worth printing where the cost differs from the obvious, or
// where there is a steer to give. The resident option is the one that stays in
// memory, and the scheduled ones are the ones that do not, which is the opposite
// of what most people expect. Daily is recommended because advisories are
// published on a scale of days, so checking more often finds the same news
// sooner than anyone can act on it while asking more of the machine.
func intervalNote(interval string) string {
	switch interval {
	case config.IntervalRealtime:
		return "  (stays resident, " + residentMemory + ", CPU is a few file checks every 10 seconds)"
	case config.IntervalDaily:
		return "  (recommended)"
	default:
		return ""
	}
}
