package service

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

// A scheduled task rather than a Windows service, because a real service has to
// be registered by an administrator and installed under an account that would
// not hold the signed-in user's credentials. A per-user task needs no elevation
// and runs as the person who scanned.
func Describe() string { return "Windows scheduled task" }

func Supported() bool {
	_, err := exec.LookPath("schtasks.exe")
	return err == nil
}

func Install(plan Plan) error {
	if !Supported() {
		return ErrUnsupported
	}

	// /end first, because /delete removes the task definition but leaves any
	// instance it started running. Without this a resident watcher survives
	// both an uninstall and an interval change, and goes on scanning under a
	// schedule that no longer exists.
	_ = exec.Command("schtasks.exe", "/end", "/tn", taskName).Run()
	_ = exec.Command("schtasks.exe", "/delete", "/tn", taskName, "/f").Run()

	command := quoteTaskPath(plan.Exec) + " watch"
	timing := taskSchedule(plan.Interval)
	realtime := plan.Interval == config.IntervalRealtime

	if realtime {
		// A resident watcher has no schedule of its own; it starts at logon and
		// stays up, doing its own waiting.
		command += " --resident"
		timing = []string{"/sc", "ONLOGON"}
	}

	args := append([]string{"/create", "/tn", taskName, "/f", "/tr", command}, timing...)
	if err := run("schtasks.exe", args...); err != nil {
		return err
	}

	if realtime {
		// ONLOGON means the next logon, which on a machine already logged in is
		// whenever it next reboots. Start it now so choosing near realtime
		// starts watching now.
		return run("schtasks.exe", "/run", "/tn", taskName)
	}
	return nil
}

func Uninstall() error {
	if !Supported() {
		return ErrUnsupported
	}
	// Stop a running instance before removing the definition, or a resident
	// watcher keeps going with nothing left to manage it.
	_ = exec.Command("schtasks.exe", "/end", "/tn", taskName).Run()
	return run("schtasks.exe", "/delete", "/tn", taskName, "/f")
}

func Status() (State, error) {
	if !Supported() {
		return State{}, ErrUnsupported
	}

	state := State{Detail: "scheduled task " + taskName}
	output, err := exec.Command("schtasks.exe", "/query", "/tn", taskName, "/fo", "list").CombinedOutput()
	if err != nil {
		return state, nil
	}

	state.Installed = true
	// The interval is deliberately not recovered from this output. Non-verbose
	// /fo LIST always carries a "Logon Mode:" field, so matching the schedule by
	// searching the whole body reported every task as near realtime whatever it
	// was really set to. The caller falls back to the saved preference, which is
	// written by the same command that created the task.
	state.Running = taskStatus(string(output))
	return state, nil
}

func run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("%s: %w", name, err)
		}
		return fmt.Errorf("%s: %s", name, trimmed)
	}
	return nil
}
