package service

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

const label = "net.stackdrift.watch"

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

	if err := writeFile(path, plist(plan)); err != nil {
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
	state.Running = exec.Command("launchctl", "list", label).Run() == nil
	return state, nil
}

func intervalFromPlist(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "<!-- stackdrift-interval="); found {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "-->"))
		}
	}
	return ""
}

func plist(plan Plan) string {
	realtime := plan.Interval == config.IntervalRealtime

	args := []string{plan.Exec, "watch"}
	schedule := fmt.Sprintf("\t<key>StartInterval</key>\n\t<integer>%d</integer>\n",
		config.IntervalSeconds(plan.Interval))
	if realtime {
		args = append(args, "--resident")
		// KeepAlive restarts the watcher if it dies, and RunAtLoad starts it
		// without waiting for the first interval that a resident process does
		// not have.
		schedule = "\t<key>KeepAlive</key>\n\t<true/>\n"
	}

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	body.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	body.WriteString("\t<!-- stackdrift-interval=" + plan.Interval + " -->\n")
	body.WriteString("\t<key>Label</key>\n\t<string>" + label + "</string>\n")
	body.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range args {
		body.WriteString("\t\t<string>" + escape(arg) + "</string>\n")
	}
	body.WriteString("\t</array>\n")
	body.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	body.WriteString(schedule)
	// Below normal priority, because nothing here is worth competing with what
	// the person at the keyboard is doing.
	body.WriteString("\t<key>Nice</key>\n\t<integer>10</integer>\n")
	body.WriteString("\t<key>LowPriorityIO</key>\n\t<true/>\n")
	body.WriteString("\t<key>ProcessType</key>\n\t<string>Background</string>\n")
	body.WriteString("</dict>\n</plist>\n")
	return body.String()
}

func escape(value string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
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
