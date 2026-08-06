package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// Swapped in tests, which cannot install a real unit
var installPlan = service.Install

// serviceAutoUpdate turns the setting on or off without disturbing anything
// else about the install.
//
// It exists because the answer is baked into the unit on Linux, where opting in
// widens the sandbox by one directory, so it cannot be flipped by editing a
// preferences file. Re-running "service install" would do it, but that also
// re-asks for the interval from a menu of three, which would silently move a
// machine off one of the intervals that is still accepted but no longer
// offered.
func serviceAutoUpdate(args []string) error {
	if !service.Supported() {
		return service.ErrUnsupported
	}

	wanted, ok := autoUpdateState(args)
	if !ok {
		return errors.New("say which: stackdrift service auto-update on|off")
	}

	state, err := service.Status()
	if err != nil {
		return err
	}
	if !state.Installed {
		return errors.New("no background service is installed, so there is nothing to keep updated")
	}

	interval := state.Interval
	if interval == "" {
		interval = config.LoadWatch().Interval
	}
	if interval == "" {
		return errors.New("could not tell how often the service runs; reinstall it with 'stackdrift service install'")
	}

	// The path comes from what is already installed, not from this process. A
	// second copy of the CLI flipping the setting must not silently repoint the
	// scheduler at itself, so an unknown path refuses rather than guessing, the
	// same way an unknown interval does above.
	exe := service.InstalledExec()
	if exe == "" {
		exe = config.LoadWatch().ServiceExec
	}
	if exe == "" {
		return errors.New("could not tell which binary the service runs; reinstall it with 'stackdrift service install'")
	}

	if err := applyServicePlan(exe, interval, &wanted); err != nil {
		return err
	}

	if wanted && appliesToInterval(interval) {
		ui.Println("Auto-update is on. The scheduled check will install a new release before it scans.")
	} else if wanted {
		ui.Println("Auto-update is on, but a resident task on this platform is never restarted, so it will not be applied.")
	} else {
		ui.Println("Auto-update is off. Update with 'stackdrift update' when you choose to.")
	}
	return nil
}

// applyServicePlan writes the scheduler entry and records the answer. Both
// halves are here so the unit and the preferences file can never disagree about
// whether auto-update is on.
//
// A nil autoUpdate means nobody answered, which is not the same as no. It keeps
// whatever the machine already had, so a scripted reinstall cannot silently
// turn off something the owner switched on.
func applyServicePlan(exe, interval string, autoUpdate *bool) error {
	effective := effectiveAutoUpdate(autoUpdate, config.LoadWatch())

	if err := installPlan(service.Plan{Interval: interval, Exec: exe, AutoUpdate: effective}); err != nil {
		return hintedError(err)
	}

	return config.UpdateWatch(func(s *config.WatchSettings) {
		s.Asked = true
		s.Enabled = true
		s.Interval = interval
		s.ServiceExec = exe
		if autoUpdate != nil {
			s.AutoUpdate = autoUpdate
		}
		// A fresh install or a deliberate change re-opens the question of
		// whether the binary can be written, and clears a stale record of the
		// last check so the new setting takes effect on the next run.
		s.UpdateBlocked = ""
		s.LastUpdateAt = ""
	})
}

// appliesToInterval reports whether an update would ever actually happen. A
// resident watcher can only take a new binary if something restarts it, so
// where nothing will, saying it is on would be a promise this cannot keep.
func appliesToInterval(interval string) bool {
	return interval != config.IntervalRealtime || service.RestartsOnExit()
}

// effectiveAutoUpdate settles what the install should actually do. Nothing
// answered means nobody was asked, which has to keep whatever the machine
// already had rather than reading as a no.
func effectiveAutoUpdate(answer *bool, settings *config.WatchSettings) bool {
	if answer != nil {
		return *answer
	}
	return settings.AutoUpdateEnabled()
}

// autoUpdateArg reads the answer from the command line for an install that
// nobody is watching. Absent stays distinguishable from an explicit no, so a
// scripted install does not answer the question on the user's behalf.
func autoUpdateArg(args []string) (bool, bool) {
	for _, arg := range args {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "--auto-update":
			return true, true
		case "--no-auto-update":
			return false, true
		}
	}
	return false, false
}

func autoUpdateState(args []string) (bool, bool) {
	for _, arg := range args {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "on", "yes", "true", "enable":
			return true, true
		case "off", "no", "false", "disable":
			return false, true
		}
	}
	return false, false
}

// askAutoUpdate puts the question at install time, where the person is already
// answering questions about the service.
//
// It is skipped without a terminal. ui.Confirm reads a closed stdin as the
// default, so a scripted "service install --interval daily" would silently
// answer yes to replacing its own binary.
func askAutoUpdate(exe string) (bool, bool) {
	if !ui.Interactive() {
		return false, false
	}

	ui.Println()
	ui.Println("Keep the StackDrift CLI itself up to date?")
	ui.Println("Each scheduled check would look for a newer release first and install it")
	ui.Println("before scanning, so this machine picks up fixes without you doing anything.")
	ui.Println()
	ui.Println("  The download comes from the public GitHub releases for the CLI. It is")
	ui.Println("  checked and run once before it replaces anything, and the version it")
	ui.Println("  displaces is kept beside it. Nothing else on the machine is touched.")

	if dir := filepath.Dir(exe); exeDirWritable(dir) != nil {
		ui.Println()
		ui.Println("  Note: " + dir + " is not writable by you, so this cannot work until the")
		ui.Println("  CLI is installed somewhere you own. Try STACKDRIFT_INSTALL_DIR=$HOME/.local/bin.")
		ui.Println()
		return ui.Confirm("Turn it on anyway?", false), true
	}

	ui.Println()
	return ui.Confirm("Turn it on?", true), true
}

// resolveAutoUpdate settles the answer for an install: the flag if one was
// given, otherwise the question. Nil means nobody answered, which the caller
// reads as "leave whatever this machine already had".
func resolveAutoUpdate(args []string) *bool {
	if answer, given := autoUpdateArg(args); given {
		return &answer
	}

	exe, err := service.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	if answer, asked := askAutoUpdate(exe); asked {
		return &answer
	}
	return nil
}

func autoUpdateStatusLines(settings *config.WatchSettings, interval string) []string {
	if !settings.AutoUpdateAnswered() {
		return []string{"  auto-update: off (never asked, turn it on with 'stackdrift service auto-update on')"}
	}
	if !settings.AutoUpdateEnabled() {
		return []string{"  auto-update: off"}
	}

	line := "  auto-update: on"
	if settings.UpdatedTo != "" {
		line += fmt.Sprintf(" (last installed %s)", settings.UpdatedTo)
	}

	// A resident watcher can only take a new binary if something restarts it,
	// so where nothing will, saying a flat "on" would be a promise this cannot
	// keep.
	if !appliesToInterval(interval) {
		line += " (not applied to a resident task on this platform)"
	}
	lines := []string{line}

	if settings.UpdateBlocked != "" {
		lines = append(lines, "  updates blocked: "+settings.UpdateBlocked)
	}
	return lines
}
