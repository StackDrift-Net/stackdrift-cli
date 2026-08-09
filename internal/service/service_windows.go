package service

import (
	"fmt"
	"os/exec"
	"strings"
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

	// The order, and which steps are allowed to fail, are settled by
	// taskInstallSteps. It is untagged so both are tested on every platform
	// rather than only on the one machine nobody runs the suite on.
	for _, step := range taskInstallSteps(plan) {
		if !step.Required {
			// Tearing the old task down, and kicking the new one off. Neither
			// says anything about whether the install worked: there is usually
			// nothing to tear down, and by the time the kick-off runs the task
			// is already registered and scheduled.
			_ = exec.Command("schtasks.exe", step.Args...).Run()
			continue
		}

		if err := run("schtasks.exe", step.Args...); err != nil {
			return err
		}
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

// InstalledExec is the binary the registered task actually runs. Empty when
// there is nothing to read.
func InstalledExec() string {
	output, err := exec.Command("schtasks.exe", "/query", "/tn", taskName, "/v", "/fo", "list").CombinedOutput()
	if err != nil {
		return ""
	}
	return execFromTask(string(output))
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
	// Both from the one probe. schtasks reports "Ready" for an armed task and
	// "Running" only during the seconds a sweep is actually executing, and both
	// words are localised, so there is no reading of this output that separates
	// armed from enabled on a machine that is not in English. Claiming to know
	// the difference is what rejected good installs abroad.
	live := taskEnabled(string(output))
	state.Enabled = live
	state.Running = live
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
