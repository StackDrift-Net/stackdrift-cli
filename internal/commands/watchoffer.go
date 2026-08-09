package commands

import (
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// Measured on the built binary, not estimated. A resident watcher settles here
// because it hands the scan's memory back after every sweep; the peaks are what
// one scan costs while it is running. The large tree figure is a 534 manifest,
// 89 GB directory, which is far past anything typical. See the README.
const (
	residentMemory = "about 12 MB"
	scanPeakMemory = "14 MB"
	largeTreePeak  = "about 30 MB"
)

// offerWatchService puts the question once per machine and remembers the answer
// either way. It is skipped entirely for --yes, because an unattended run has
// nobody to answer and a CI job must not end up installing a service on a build
// agent.
func offerWatchService(assumeYes bool) {
	if assumeYes || !service.Supported() {
		return
	}

	settings := config.LoadWatch()
	if settings.Asked {
		// Answered once and never asked again, which is right, but it also
		// meant a service that stopped being scheduled stayed that way for
		// ever. A scan is the only thing still running on such a machine, so it
		// is the only thing that can put it back.
		repairWatchService(settings)
		return
	}

	// One service covers every directory scanned on this machine, so a machine
	// that already has one has answered this. Only reached when the preferences
	// file is missing or damaged, and putting the offer there would end in
	// "Not installed" printed over a service that is still running.
	if state, err := service.Status(); err == nil && state.Installed {
		adoptInstalledService(state)

		// Adopting only records that this machine is meant to be covered, which
		// is not the same as being covered. This branch is reached when the
		// preferences file is missing or damaged, and a machine in that state is
		// just as likely to have the unit files with nothing scheduling them,
		// which is exactly the fault everything here exists to catch. Repairing
		// on the same run means it is fixed now rather than at the next scan.
		repairWatchService(config.LoadWatch())
		return
	}

	ui.Println()
	ui.Println("Keep this system up to date automatically?")
	ui.Println("StackDrift can install a background " + serviceNoun() + " that notices when what you")
	ui.Println("already track here moves: a package upgrade, a rewritten lock file, a new")
	ui.Println("kernel or OS release. It updates the system for you, so the advisories")
	ui.Println("you get are about the versions actually installed.")
	ui.Println()
	ui.Println("  It only ever updates what this system already tracks, and only where")
	ui.Println("  the evidence is unambiguous. It never adds software you have not chosen,")
	ui.Println("  and the only row it removes is one a version change has replaced. Run")
	ui.Println("  scan again yourself when you install something new.")
	ui.Println()
	describeCost()
	ui.Println()

	if !ui.Confirm("Install it?", true) {
		// Recorded as asked so the offer is not put again after every scan.
		// Changing their mind is one command, which the decline says.
		_ = config.UpdateWatch(func(s *config.WatchSettings) {
			s.Asked = true
			s.Enabled = false
		})
		ui.Println("Not installed. Run 'stackdrift service install' if you change your mind.")
		return
	}

	if !confirmNoDoubleUp() {
		ui.Println("Nothing installed. Run 'stackdrift service install' if you change your mind.")
		return
	}

	interval, ok := askInterval()
	if !ok {
		ui.Println("No valid choice made, so nothing was installed.")
		ui.Println("Run 'stackdrift service install' to try again.")
		return
	}

	if err := installService(interval, resolveAutoUpdate(nil)); err != nil {
		// A failed install must not be recorded as a decision, or the offer
		// never comes back and the user is left with nothing watching.
		ui.Println("Could not install the service: " + err.Error())
		ui.Println("Run 'stackdrift service install' to try again.")
	}
}

// adoptInstalledService writes back what the platform already has, so the next
// scan reads the answer out of the preferences file rather than asking the
// scheduler again. The interval comes from the installed unit, which is the
// only record of it once the file it was saved to has gone.
func adoptInstalledService(state service.State) {
	_ = config.UpdateWatch(func(s *config.WatchSettings) {
		s.Asked = true
		s.Enabled = true
		s.Interval = state.Interval
	})
}

// Every interval on offer is handed to the platform's own scheduler, so there
// is one cost to quote rather than one per mode.
func describeCost() {
	ui.Println("  What it costs, measured:")
	ui.Println("    Nothing runs at all between checks. Each check finishes in well under")
	ui.Println("    a second and peaks around " + scanPeakMemory + ".")
	ui.Println()
	ui.Println("  A very large directory can push one check to " + largeTreePeak + " for a moment. It")
	ui.Println("  runs at low priority and idle IO, so it yields to everything else.")
}

func serviceNoun() string {
	switch service.Describe() {
	case "systemd user service":
		return "service"
	case "launchd agent":
		return "agent"
	default:
		return "task"
	}
}
