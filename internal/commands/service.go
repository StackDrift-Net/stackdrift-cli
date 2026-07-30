package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/service"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

// The order they are offered in, cheapest attention first. Realtime leads
// because it is the one people want and the one whose cost needs explaining.
var intervalChoices = []string{
	config.IntervalRealtime,
	config.IntervalFiveMin,
	config.IntervalHourly,
	config.IntervalTwiceDay,
	config.IntervalDaily,
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
	default:
		return fmt.Errorf("unknown service action %q, expected install, uninstall or status", action)
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

	return installService(interval)
}

func installService(interval string) error {
	exe, err := service.Executable()
	if err != nil {
		return err
	}

	if err := service.Install(service.Plan{Interval: interval, Exec: exe}); err != nil {
		return err
	}

	if err := config.SaveWatch(&config.WatchSettings{
		Asked:    true,
		Enabled:  true,
		Interval: interval,
	}); err != nil {
		return err
	}

	ui.Println()
	ui.Println("Installed the " + service.Describe() + ", running " + config.IntervalLabel(interval) + ".")
	ui.Println("Stop it any time with: stackdrift service uninstall")
	return nil
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
	if err := config.SaveWatch(&config.WatchSettings{Asked: true, Enabled: false}); err != nil {
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

	if !state.Installed {
		ui.Println("No background service is installed.")
		ui.Println("Install one with: stackdrift service install")
		return nil
	}

	interval := state.Interval
	if interval == "" {
		interval = config.LoadWatch().Interval
	}

	ui.Println("Background service: " + service.Describe())
	if state.Detail != "" {
		ui.Println("  unit:     " + state.Detail)
	}
	if interval != "" {
		ui.Println("  interval: " + config.IntervalLabel(interval))
	}
	if state.Running {
		ui.Println("  state:    running")
	} else {
		ui.Println("  state:    installed but not running")
	}
	return nil
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
	case config.IntervalWeekly:
		return "Weekly"
	default:
		return interval
	}
}

// The note is only worth printing where the cost differs from the obvious. The
// resident option is the one that stays in memory, and the scheduled ones are
// the ones that do not, which is the opposite of what most people expect.
func intervalNote(interval string) string {
	if interval == config.IntervalRealtime {
		return "  (stays resident, " + residentMemory + ", CPU is a few file checks every 10 seconds)"
	}
	return ""
}
