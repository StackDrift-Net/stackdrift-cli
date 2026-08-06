package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// The shortest gap between two release-feed calls, whatever the watch interval
// is. Every interval on offer is longer, so it exists for the shorter ones that
// --interval still accepts and that are already running in the field, where a
// check per sweep would call GitHub every five minutes for ever.
const updateCheckFloor = 12 * time.Hour

// autoUpdateWanted answers whether this run may replace the binary. Every
// clause is a way of being sure nobody is surprised by it. Only the run the
// scheduler starts carries the answer given at install time. Absent is not no,
// so installs predating the question are left alone. The child of an update
// must not update, or a build the feed keeps calling behind would loop. And a
// binary already known to be unreplaceable stops being checked.
func autoUpdateWanted(scheduled bool, settings *config.WatchSettings, upgraded bool) bool {
	if !scheduled || upgraded {
		return false
	}
	if !settings.AutoUpdateEnabled() {
		return false
	}
	return settings.UpdateBlocked == ""
}

func dueForUpdateCheck(last string, now time.Time) bool {
	if last == "" {
		return true
	}

	stamp, err := time.Parse(time.RFC3339, last)
	if err != nil {
		// A damaged stamp must not wedge the check off for ever, which reading
		// it as "checked just now" would do.
		return true
	}

	// A stamp ahead of the clock is a machine whose time went backwards or a
	// file copied from another one. Waiting it out could mean waiting years.
	if stamp.After(now) {
		return true
	}
	return now.Sub(stamp) >= updateCheckFloor
}

// shouldReplace decides whether the release is worth fetching.
//
// updatedTo is what was last written over the executable, which is NOT the
// version this process reports. A watcher whose scheduler will not restart it
// keeps running the old code after a successful replace, so without this it
// would fetch the same release every day for ever. It is also the only thing
// that stops a dev build looping, since needsUpdate treats dev as permanently
// behind.
func shouldReplace(current, latest, updatedTo string) bool {
	if !needsUpdate(current, latest) {
		return false
	}
	return normalizeVersion(updatedTo) != normalizeVersion(latest)
}

// exeDirWritable answers by trying, not by reading permission bits. Under
// systemd what refuses the write is a read-only mount from ProtectSystem=strict,
// which no permission bit shows.
func exeDirWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".stackdrift-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

// selfUpdate runs before the sweep it belongs to, so a build the server would
// refuse has already replaced itself before it asks the server anything.
//
// It never returns an error. An update that cannot happen is not a reason to
// skip the sweep, which is the thing the machine is actually for.
func selfUpdate(scheduled bool) (rerun bool, installed string) {
	settings := config.LoadWatch()
	if !autoUpdateWanted(scheduled, settings, AlreadyUpgraded()) {
		return false, ""
	}
	if !dueForUpdateCheck(settings.LastUpdateAt, time.Now().UTC()) {
		return false, ""
	}

	exe, err := currentExecutable()
	if err != nil {
		return false, ""
	}

	if err := exeDirWritable(filepath.Dir(exe)); err != nil {
		ui.Println("Auto-update is on, but " + filepath.Dir(exe) + " cannot be written: " + err.Error())

		// Only a cause that cannot clear itself is remembered. A full disk or a
		// momentarily missing directory would otherwise turn auto-update off
		// for the life of the install with no sign of why.
		if permanentlyUnwritable(err) {
			ui.Println("Nothing will be replaced. Reinstall the CLI somewhere writable, or turn this off with 'stackdrift service auto-update off'.")
			_ = config.UpdateWatch(func(s *config.WatchSettings) { s.UpdateBlocked = err.Error() })
			return false, ""
		}

		stamped := time.Now().UTC().Format(time.RFC3339)
		_ = config.UpdateWatch(func(s *config.WatchSettings) { s.LastUpdateAt = stamped })
		return false, ""
	}

	// Stamped whether or not anything is replaced, and before the outcome is
	// known, so a release feed that is down does not produce one attempt per
	// sweep.
	stamped := time.Now().UTC().Format(time.RFC3339)
	_ = config.UpdateWatch(func(s *config.WatchSettings) { s.LastUpdateAt = stamped })

	latest, err := fetchLatestTag(updateBase("STACKDRIFT_UPDATE_API", "https://api.github.com"), releaseRepo)
	if err != nil {
		ui.Println("Update check skipped: " + err.Error())
		return false, ""
	}

	if !shouldReplace(api.Version, latest, settings.UpdatedTo) {
		return false, ""
	}

	installed, err = fetchVerifyReplace(updateBase("STACKDRIFT_UPDATE_DOWNLOAD", "https://github.com"), latest)
	if err != nil {
		ui.Println("Update to " + latest + " failed: " + err.Error())
		return false, ""
	}

	_ = config.UpdateWatch(func(s *config.WatchSettings) { s.UpdatedTo = normalizeVersion(latest) })
	ui.Println("Updated to " + latest + ".")
	return true, installed
}

// residentHandover replaces the binary of a watcher that stays running, and
// reports whether this process should now stand down for the scheduler to start
// the new one.
//
// Where nothing will start it again, the update is skipped rather than done and
// left dormant: the replaced binary would sit there with nothing to run it, and
// on Windows it would hold the displaced copy locked as well.
func residentHandover() bool {
	if !service.RestartsOnExit() {
		return false
	}
	handover, _ := selfUpdate(true)
	return handover
}

// rerunAfterUpdate runs the same command again with the binary that has just
// replaced this one, and reports what to finish on.
func rerunAfterUpdate(self string, args []string) *ExitCodeError {
	code, err := RerunSelf(self, args, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: updated, but could not re-run the command: "+err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}
	return &ExitCodeError{Code: code}
}
