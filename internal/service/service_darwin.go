package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// A LaunchAgent, not a LaunchDaemon, for the same reason Linux uses a user
// unit: the credentials the scan needs are in the user's own config directory.
func Describe() string { return "launchd agent" }

func Supported() bool {
	_, err := exec.LookPath("launchctl")
	return err == nil
}

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func Install(plan Plan) error {
	if !Supported() {
		return ErrUnsupported
	}

	path, err := plistPath()
	if err != nil {
		return err
	}

	// Replacing a loaded agent in place leaves launchd running the old command,
	// so it is unloaded first and the failure ignored when nothing was loaded.
	_ = bootout(path)

	if err := writeFile(path, plistBody(plan)); err != nil {
		return err
	}
	return bootstrap(path)
}

func Uninstall() error {
	if !Supported() {
		return ErrUnsupported
	}

	path, err := plistPath()
	if err != nil {
		return err
	}

	_ = bootout(path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// InstalledExec is the binary the installed agent actually starts. Empty when
// there is nothing to read.
func InstalledExec() string {
	path, err := plistPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return execFromPlist(string(data))
}

func Status() (State, error) {
	if !Supported() {
		return State{}, ErrUnsupported
	}

	path, err := plistPath()
	if err != nil {
		return State{}, err
	}

	state := State{Detail: "launchd agent " + label}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, nil
	}

	state.Installed = true
	state.Interval = intervalFromPlist(string(data))

	// launchd draws no line between armed and enabled for an interval agent.
	// Either it has been bootstrapped into the user's domain, in which case it
	// is both, or it has not, in which case it is neither. The two flags are
	// reported from the one probe rather than inventing a distinction the
	// platform does not have.
	loaded := exec.Command("launchctl", "list", label).Run() == nil
	state.Enabled = loaded
	state.Running = loaded
	return state, nil
}

func bootstrap(path string) error {
	target := "gui/" + strconv.Itoa(os.Getuid())
	if err := run("launchctl", "bootstrap", target, path); err == nil {
		return nil
	}
	// bootstrap arrived in macOS 10.11 and load is what works before it, so the
	// older verb is the fallback rather than the failure.
	return run("launchctl", "load", "-w", path)
}

func bootout(path string) error {
	target := "gui/" + strconv.Itoa(os.Getuid())
	if err := exec.Command("launchctl", "bootout", target+"/"+label).Run(); err == nil {
		return nil
	}
	return exec.Command("launchctl", "unload", "-w", path).Run()
}

func run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), trimmed)
	}
	return nil
}
