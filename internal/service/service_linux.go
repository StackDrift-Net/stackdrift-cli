package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

// A user unit rather than a system one, because the credentials the scan needs
// live under the user's own config directory and a root service would not find
// them. Lingering is what keeps a user unit alive on a server nobody is logged
// in to, which is the case this feature exists for.
func Describe() string { return "systemd user service" }

func Supported() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func unitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func Install(plan Plan) error {
	if !Supported() {
		return ErrUnsupported
	}

	dir, err := unitDir()
	if err != nil {
		return err
	}

	realtime := plan.Interval == config.IntervalRealtime

	// Whatever is installed now is stopped before anything is rewritten. Both
	// directions need it. Switching down from an interval leaves a timer that
	// would keep starting a unit which now never exits; switching up from
	// realtime leaves a resident watcher running the old command, and because
	// daemon-reload neither stops nor restarts an active unit, the new timer's
	// attempts to start it are no-ops and the chosen schedule never runs at all.
	_ = disableUnit(Name + ".timer")
	_ = disableUnit(Name + ".service")

	if err := writeFile(filepath.Join(dir, Name+".service"), unitFile(plan, realtime)); err != nil {
		return err
	}

	timer := filepath.Join(dir, Name+".timer")
	if realtime {
		_ = os.Remove(timer)
	} else if err := writeFile(timer, timerFile(plan)); err != nil {
		return err
	}

	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}

	// Without lingering the unit dies at logout, which on a server is most of
	// the time. A failure here is not fatal: the service still works while the
	// user is logged in, and saying so beats refusing to install.
	if user := os.Getenv("USER"); user != "" {
		_ = run("loginctl", "enable-linger", user)
	}

	unit := Name + ".timer"
	if realtime {
		unit = Name + ".service"
	}
	return run("systemctl", "--user", "enable", "--now", unit)
}

func Uninstall() error {
	if !Supported() {
		return ErrUnsupported
	}

	dir, err := unitDir()
	if err != nil {
		return err
	}

	_ = disableUnit(Name + ".timer")
	_ = disableUnit(Name + ".service")
	_ = os.Remove(filepath.Join(dir, Name+".timer"))
	_ = os.Remove(filepath.Join(dir, Name+".service"))

	return run("systemctl", "--user", "daemon-reload")
}

func Status() (State, error) {
	if !Supported() {
		return State{}, ErrUnsupported
	}

	dir, err := unitDir()
	if err != nil {
		return State{}, err
	}

	state := State{}
	serviceUnit := filepath.Join(dir, Name+".service")
	if _, err := os.Stat(serviceUnit); err == nil {
		state.Installed = true
	}

	timerUnit := filepath.Join(dir, Name+".timer")
	_, timerErr := os.Stat(timerUnit)
	if timerErr == nil {
		state.Running = active(Name + ".timer")
		state.Detail = "systemd timer " + Name + ".timer"
	} else if state.Installed {
		state.Running = active(Name + ".service")
		state.Detail = "systemd service " + Name + ".service"
	}

	if data, err := os.ReadFile(serviceUnit); err == nil {
		state.Interval = intervalFromUnit(string(data))
	}
	return state, nil
}

// The interval is read back out of the unit rather than trusted from the saved
// preferences, so a status command reports what is actually installed even when
// the two have drifted apart.
func intervalFromUnit(unit string) string {
	for _, line := range strings.Split(unit, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "# stackdrift-interval="); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func unitFile(plan Plan, realtime bool) string {
	var body strings.Builder
	body.WriteString("[Unit]\n")
	body.WriteString("Description=StackDrift stack change watcher\n")
	body.WriteString("Documentation=https://stackdrift.net\n")
	body.WriteString("After=network-online.target\n\n")
	body.WriteString("# stackdrift-interval=" + plan.Interval + "\n")
	body.WriteString("[Service]\n")

	if realtime {
		body.WriteString("Type=simple\n")
		body.WriteString(fmt.Sprintf("ExecStart=%s watch --resident\n", quote(plan.Exec)))
		// A watcher that gives up on the first network blip is worse than no
		// watcher, because nothing says it stopped.
		body.WriteString("Restart=always\n")
		body.WriteString("RestartSec=30\n")
	} else {
		body.WriteString("Type=oneshot\n")
		body.WriteString(fmt.Sprintf("ExecStart=%s watch\n", quote(plan.Exec)))
	}

	// The scan reads the machine and talks to one host. Nothing it does needs
	// to write outside the user's own state, so the unit says so.
	//
	// PrivateTmp is deliberately not set. It would give the unit a different
	// /tmp from the one the person scanning saw, so a project path under /tmp
	// would be scanned as an empty directory and quietly report nothing. It
	// also breaks outright, with a bare 203/EXEC, when the binary itself lives
	// there. This has to see the same filesystem the scan did.
	body.WriteString("Nice=10\n")
	body.WriteString("IOSchedulingClass=idle\n")
	body.WriteString("NoNewPrivileges=true\n")
	body.WriteString("ProtectSystem=strict\n")
	body.WriteString("ProtectHome=read-only\n")
	body.WriteString("ReadWritePaths=%h/.stackdrift\n\n")

	if realtime {
		body.WriteString("[Install]\nWantedBy=default.target\n")
	}
	return body.String()
}

func timerFile(plan Plan) string {
	seconds := config.IntervalSeconds(plan.Interval)
	// Monotonic rather than OnCalendar, because the interval means "every N"
	// rather than "at this time". Persistent= is deliberately absent: it only
	// has an effect on OnCalendar timers, so setting it here would do nothing
	// while reading as though missed runs were covered. OnBootSec is what
	// actually covers a machine that was switched off, by running shortly after
	// it comes back rather than waiting a full interval.
	return fmt.Sprintf(`[Unit]
Description=StackDrift stack change watcher (%s)

[Timer]
OnBootSec=2min
OnUnitActiveSec=%ds
# Spreads the wake-up so every machine on one schedule does not call at once.
RandomizedDelaySec=%ds
Unit=%s.service

[Install]
WantedBy=timers.target
`, config.IntervalLabel(plan.Interval), seconds, jitter(seconds), Name)
}

func jitter(seconds int) int {
	spread := seconds / 10
	if spread > 300 {
		spread = 300
	}
	if spread < 1 {
		spread = 1
	}
	return spread
}

func active(unit string) bool {
	return exec.Command("systemctl", "--user", "is-active", "--quiet", unit).Run() == nil
}

func disableUnit(unit string) error {
	return run("systemctl", "--user", "disable", "--now", unit)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), trimmed)
	}
	return nil
}

// systemd splits ExecStart on spaces unless the argument is quoted, so a binary
// installed under a path with a space in it would otherwise never start.
func quote(value string) string {
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
