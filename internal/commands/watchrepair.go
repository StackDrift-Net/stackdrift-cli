package commands

import (
	"errors"
	"io/fs"
	"os"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// Swapped in tests, because both of these read the machine's real scheduler
var (
	statusOf        = service.Status
	installedExecOf = service.InstalledExec
)

// repairWatchService puts back a schedule that has stopped being one.
//
// It has to run from an interactive command, and that is the whole point. The
// scheduled run cannot repair itself, because a scheduler that is not holding
// the unit is exactly the fault being repaired and no scheduled run will ever
// happen to notice it. Scans are the only thing that still runs on a machine in
// that state.
//
// Only ever acts where the saved preference says a service is wanted. Someone
// who declined, or who ran uninstall, has Enabled false and must never have one
// put back under them.
func repairWatchService(settings *config.WatchSettings) {
	if settings == nil || !settings.Enabled || !service.Supported() {
		return
	}

	state, err := statusOf()
	if err != nil {
		// The platform could not be asked. Reinstalling on the strength of an
		// answer nobody got would reinstall on every scan for ever.
		return
	}

	exe := installedExecOf()
	reason := repairReason(state, exe, exe != "" && exists(exe))
	if reason == "" {
		return
	}

	ui.Println()
	ui.Println("The background " + serviceNoun() + " had stopped being scheduled: " + reason + ".")

	interval := settings.Interval
	if !config.KnownInterval(interval) {
		// Whatever it was set to is unreadable or gone, and leaving it empty
		// would install a unit with no schedule at all.
		interval = config.IntervalDaily
	}

	// The binary the scheduler already points at is kept, and this process is
	// used only when that one has gone.
	//
	// A repair is not an invitation to repoint the scheduler. Someone
	// diagnosing this is quite likely running a copy from a downloads folder,
	// and installing THAT path would leave every scheduled run failing with
	// 203/EXEC the moment they deleted it. serviceautoupdate.go already refuses
	// to guess here for the same reason.
	current := exe
	if current == "" || !exists(current) {
		resolved, err := service.Executable()
		if err != nil {
			ui.Println("Could not repair it: " + err.Error())
			return
		}
		current = resolved
	}

	// nil keeps whatever they answered about auto-update. A repair is not a
	// place to quietly change a decision they already made.
	if err := applyServicePlan(current, interval, nil); err != nil {
		ui.Println("Could not repair it: " + err.Error())
		ui.Println("Run 'stackdrift service install' to try again.")
		return
	}

	ui.Println("Repaired it, running " + config.IntervalLabel(interval) + ".")
}

// repairReason names what is wrong, or is empty when nothing is. Every branch
// here is a state a real machine was found in.
//
// execExists is passed in rather than read here so the decision stays pure. It
// is only meaningful when installedExec is non-empty: Windows reads its task
// back through a localised field it cannot always parse, so an empty answer
// there means "do not know" and must never be read as "the binary has gone".
func repairReason(state service.State, installedExec string, execExists bool) string {
	switch {
	case !state.Installed:
		return "the unit it was installed as is no longer there"
	case !state.Enabled:
		return "it is installed but was never enabled, so it has never run"
	case !state.Running:
		return "the scheduler is not holding it"
	case installedExec != "" && !execExists:
		return "it points at a binary that is no longer there"
	default:
		return ""
	}
}

// exists answers "is this binary still there", and a path it is not allowed to
// look at counts as still there.
//
// Only an explicit "no such file" is absence. Anything else is ignorance, and
// the difference decides whether a repair fires. On Windows the scheduled task
// is registered once for the whole machine, so it can name a binary under
// another account's profile, which this process cannot traverse. Reading that
// permission error as a deleted binary made every interactive scan quietly take
// the task over from the other account, and take it back again the next time
// they scanned.
func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}
