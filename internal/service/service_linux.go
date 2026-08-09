package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
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

// Swapped in tests to observe the order of the calls Install makes
var runCommand = run

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

	// Before anything needs the user bus, loginctl reaches logind over the
	// system bus so it works when the user bus is what is missing
	if user := os.Getenv("USER"); user != "" {
		_ = runCommand("loginctl", "enable-linger", user)
	}

	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}

	unit := Name + ".timer"
	if realtime {
		unit = Name + ".service"
	}
	return runCommand("systemctl", "--user", "enable", "--now", unit)
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
		state.Enabled = unitEnabled(Name + ".timer")
		state.Running = active(Name + ".timer")
		state.Detail = "systemd timer " + Name + ".timer"
	} else if state.Installed {
		state.Enabled = unitEnabled(Name + ".service")
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

// InstalledExec is the binary the installed unit actually starts, so a setting
// can be changed without repointing the scheduler at whichever copy of the CLI
// happened to run the command. Empty when there is nothing to read.
func InstalledExec() string {
	dir, err := unitDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, Name+".service"))
	if err != nil {
		return ""
	}
	return execFromUnit(string(data))
}

func execFromUnit(unit string) string {
	for _, line := range strings.Split(unit, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "ExecStart=")
		if !found {
			continue
		}
		return unquote(firstField(strings.TrimSpace(rest)))
	}
	return ""
}

// The path is the first argument, and it is quoted when it holds a space, so
// splitting on spaces alone would cut such a path in half. quote escapes an
// embedded quote, so the closing one has to be found past any escape or the
// path is truncated and then written back truncated.
func firstField(command string) string {
	if !strings.HasPrefix(command, `"`) {
		if space := strings.IndexAny(command, " \t"); space >= 0 {
			return command[:space]
		}
		return command
	}

	for i := 1; i < len(command); i++ {
		if command[i] == '\\' {
			i++
			continue
		}
		if command[i] == '"' {
			return command[:i+1]
		}
	}
	return command
}

func unquote(value string) string {
	if len(value) < 2 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
		return value
	}

	var out strings.Builder
	body := value[1 : len(value)-1]
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
		}
		out.WriteByte(body[i])
	}
	return out.String()
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
		body.WriteString(fmt.Sprintf("ExecStart=%s watch --resident %s\n", quote(plan.Exec), ScheduledFlag))
		// A watcher that gives up on the first network blip is worse than no
		// watcher, because nothing says it stopped.
		body.WriteString("Restart=always\n")
		body.WriteString("RestartSec=30\n")
	} else {
		body.WriteString("Type=oneshot\n")
		body.WriteString(fmt.Sprintf("ExecStart=%s watch %s\n", quote(plan.Exec), ScheduledFlag))
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
	body.WriteString("ReadWritePaths=%h/.stackdrift" + writableExecDir(plan) + "\n\n")

	if realtime {
		body.WriteString("[Install]\nWantedBy=default.target\n")
	}
	return body.String()
}

// writableExecDir opens the sandbox by exactly one directory, and only for
// someone who asked for auto-update.
//
// ProtectSystem=strict and ProtectHome=read-only leave the whole filesystem
// read-only apart from the link store, which includes wherever the binary is
// installed. Without this the update fails with "Read-only file system" before
// a byte is downloaded, on every run, for ever. Verified against real systemd
// both ways round.
//
// Prefixed with a dash so a directory that has since been removed leaves the
// update failing rather than stopping the whole watcher from starting.
func writableExecDir(plan Plan) string {
	if !plan.AutoUpdate {
		return ""
	}

	dir := filepath.Dir(plan.Exec)
	if dir == "" || dir == "." || dir == "/" {
		return ""
	}
	return " -" + quote(dir)
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
	return command("systemctl", "--user", "is-active", "--quiet", unit).Run() == nil
}

// unitEnabled asks whether the manager will pull the unit in on its own.
//
// Only ever called for the unit that carries the [Install] section, which is
// the timer on a schedule and the service in realtime. is-enabled exits zero
// for "static" as well, so calling it on the oneshot service behind a timer
// would answer yes about a unit nothing is scheduling.
func unitEnabled(unit string) bool {
	return command("systemctl", "--user", "is-enabled", "--quiet", unit).Run() == nil
}

// Fills XDG_RUNTIME_DIR when the caller has none
// sd-bus derives the user bus address from it alone, so ssh, cron and su fail
// with ENOMEDIUM even when the socket is sitting there
//
// The bus address goes with it. Supplying the directory and leaving that
// variable behind fixes nothing, because sd-bus reads it first and honours it
// even when it is empty or names a bus belonging to whoever was logged in
// before a su. The directory is then never consulted and the call still fails,
// which is how installs over ssh managed to write their unit files and schedule
// nothing at all.
func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)

	// The two corrections are decided separately, because they answer different
	// questions and welding them together left a real case broken.
	//
	// The bus address is dropped whenever this user has a runtime directory of
	// their own, not only when we are also replacing the variable. systemctl
	// tries $XDG_RUNTIME_DIR/systemd/private first and ignores the bus address
	// entirely when that connects, so on a healthy machine this costs nothing.
	// When that socket is unusable it falls back to sd_bus_default_user, which
	// prefers the variable over the directory, so a caller whose directory was
	// ALREADY correct but who carried a stale address still failed. Verified by
	// strace: connect(/run/user/1000/systemd/private) ECONNREFUSED, then
	// connect(/run/user/0/bus) ENOENT.
	exists := runtimeDirExists()
	if !exists {
		return cmd
	}

	cmd.Env = withoutBusAddress(os.Environ())
	if entry := runtimeDirEnv(os.Getenv("XDG_RUNTIME_DIR"), defaultRuntimeDir(), exists); entry != "" {
		cmd.Env = append(cmd.Env, entry)
	}
	return cmd
}

// Only ever called alongside a supplied XDG_RUNTIME_DIR, so a caller with a
// working session of their own keeps whatever they set.
func withoutBusAddress(environ []string) []string {
	const key = "DBUS_SESSION_BUS_ADDRESS="

	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		if strings.HasPrefix(entry, key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func defaultRuntimeDir() string {
	return "/run/user/" + strconv.Itoa(os.Getuid())
}

func runtimeDirExists() bool {
	info, err := os.Stat(defaultRuntimeDir())
	return err == nil && info.IsDir()
}

// runtimeDirEnv decides whether to supply XDG_RUNTIME_DIR, returning empty for
// leave it alone.
//
// It corrects two cases, and the second one is not obvious. Missing is the easy
// one: ssh, cron and a bare su leave it unset. The other is a value inherited
// from a DIFFERENT user, which is what "su ubuntu" from a root shell produces,
// because non-login su passes only HOME, SHELL, USER and LOGNAME through and
// root's /run/user/0 survives into a process running as uid 1000. Every
// systemctl --user call then aims at the wrong manager and fails.
//
// That case used to be left alone on the grounds that the caller had set it
// deliberately. Nobody deliberately points at another user's runtime directory.
// It is inherited, it is always wrong, and treating it as a considered choice
// is how two servers ended up with unit files and no schedule.
func runtimeDirEnv(current, dir string, exists bool) string {
	if !exists {
		return ""
	}
	// Only an exact match is left alone. Anything else is rewritten to the
	// canonical path, which costs nothing and closes the hole where a value
	// that merely LOOKS equal after trimming is passed through unchanged and
	// then fails, whitespace and all, in the child.
	if current == dir {
		return ""
	}
	return "XDG_RUNTIME_DIR=" + dir
}

func disableUnit(unit string) error {
	return runCommand("systemctl", "--user", "disable", "--now", unit)
}

func run(name string, args ...string) error {
	cmd := command(name, args...)
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
