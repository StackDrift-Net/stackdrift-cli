package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestUnitFile_Realtime_StaysRunningAndRestarts(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalRealtime, Exec: "/usr/local/bin/stackdrift"}, true)

	if !strings.Contains(unit, "Type=simple") {
		t.Fatalf("a resident watcher is not a oneshot:\n%s", unit)
	}
	if !strings.Contains(unit, "watch --resident") {
		t.Fatalf("expected the resident flag passed:\n%s", unit)
	}
	if !strings.Contains(unit, "Restart=always") {
		t.Fatalf("a watcher that dies silently is worse than none:\n%s", unit)
	}
}

func TestUnitFile_Scheduled_RunsOnceAndExits(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalHourly, Exec: "/usr/local/bin/stackdrift"}, false)

	if !strings.Contains(unit, "Type=oneshot") {
		t.Fatalf("a timed sweep must exit so nothing is resident between runs:\n%s", unit)
	}
	if strings.Contains(unit, "--resident") {
		t.Fatalf("a timed sweep must not stay running:\n%s", unit)
	}
}

// Only a unit that starts itself carries an Install section. A timer-driven
// oneshot enabled directly would run once at boot and never again.
func TestUnitFile_Scheduled_IsNotEnabledOnItsOwn(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "[Install]") {
		t.Fatalf("the timer owns the schedule, not the service:\n%s", unit)
	}
}

func TestUnitFile_Realtime_IsEnabledOnItsOwn(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalRealtime, Exec: "/x/stackdrift"}, true)

	if !strings.Contains(unit, "[Install]") {
		t.Fatalf("a resident watcher has no timer to start it:\n%s", unit)
	}
}

func TestUnitFile_Always_YieldsToEverythingElse(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if !strings.Contains(unit, "Nice=10") || !strings.Contains(unit, "IOSchedulingClass=idle") {
		t.Fatalf("a background scan must not compete with real work:\n%s", unit)
	}
}

// The scan reads the machine and writes only its own link store, so anything
// more than that is authority it does not need.
func TestUnitFile_Always_CanOnlyWriteItsOwnStore(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if !strings.Contains(unit, "ProtectSystem=strict") {
		t.Fatalf("expected the filesystem protected:\n%s", unit)
	}
	if !strings.Contains(unit, "ReadWritePaths=%h/.stackdrift") {
		t.Fatalf("expected exactly one writable path:\n%s", unit)
	}
	if !strings.Contains(unit, "NoNewPrivileges=true") {
		t.Fatalf("expected privilege escalation refused:\n%s", unit)
	}
}

// A private /tmp would be a different /tmp from the one the person scanning
// saw, so a project path under it would be scanned as an empty directory and
// report nothing at all. The unit has to see the filesystem the scan did.
func TestUnitFile_Always_SharesTheRealTmp(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "PrivateTmp") {
		t.Fatalf("the unit must see the same /tmp the scan did:\n%s", unit)
	}
}

func TestUnitFile_Always_RecordsTheIntervalItWasBuiltFor(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalTwiceDay, Exec: "/x/stackdrift"}, false)

	if got := intervalFromUnit(unit); got != config.IntervalTwiceDay {
		t.Fatalf("status reads the interval back out of the unit, got %q", got)
	}
}

func TestIntervalFromUnit_NoMarker_IsEmpty(t *testing.T) {
	if got := intervalFromUnit("[Service]\nExecStart=/x/stackdrift watch\n"); got != "" {
		t.Fatalf("expected nothing for a unit with no marker, got %q", got)
	}
}

func TestTimerFile_Always_UsesTheIntervalSeconds(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalFiveMin})

	if !strings.Contains(timer, "OnUnitActiveSec=300s") {
		t.Fatalf("expected the five minute gap:\n%s", timer)
	}
}

// A machine that was switched off has to be scanned soon after it comes back
// rather than waiting out a whole interval, which is what OnBootSec buys.
func TestTimerFile_Always_RunsSoonAfterBoot(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalDaily})

	if !strings.Contains(timer, "OnBootSec=") {
		t.Fatalf("a machine that was off must not wait a full interval:\n%s", timer)
	}
}

// Persistent= only has an effect on OnCalendar timers. Setting it on a
// monotonic one does nothing at all while reading as though missed runs were
// covered, which is worse than not claiming it.
func TestTimerFile_MonotonicTimer_DoesNotClaimPersistence(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalDaily})

	if strings.Contains(timer, "Persistent=") && !strings.Contains(timer, "OnCalendar=") {
		t.Fatalf("Persistent= without OnCalendar= is a no-op that reads as a guarantee:\n%s", timer)
	}
}

func TestTimerFile_Always_StartsTheServiceUnit(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalWeekly})

	if !strings.Contains(timer, "Unit="+Name+".service") {
		t.Fatalf("expected the timer bound to the service:\n%s", timer)
	}
}

func TestJitter_ShortInterval_StaysWellUnderIt(t *testing.T) {
	if spread := jitter(300); spread >= 300 {
		t.Fatalf("a spread of %ds would double a five minute interval", spread)
	}
}

// Every machine on a weekly schedule waking at the same second would arrive as
// one burst, so the spread is capped rather than proportional forever.
func TestJitter_LongInterval_IsCapped(t *testing.T) {
	if spread := jitter(604800); spread != 300 {
		t.Fatalf("expected the spread capped at 300s, got %d", spread)
	}
}

func TestJitter_TinyInterval_IsStillPositive(t *testing.T) {
	if spread := jitter(5); spread < 1 {
		t.Fatalf("RandomizedDelaySec must be positive, got %d", spread)
	}
}

func TestQuote_PathWithASpace_IsQuoted(t *testing.T) {
	if got := quote("/home/a b/stackdrift"); got != `"/home/a b/stackdrift"` {
		t.Fatalf("systemd splits on spaces, got %s", got)
	}
}

func TestQuote_PlainPath_IsLeftAlone(t *testing.T) {
	if got := quote("/usr/local/bin/stackdrift"); got != "/usr/local/bin/stackdrift" {
		t.Fatalf("expected no quoting, got %s", got)
	}
}

func TestUnitFile_PathWithASpace_QuotesTheExec(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/opt/my tools/stackdrift"}, false)

	if !strings.Contains(unit, `ExecStart="/opt/my tools/stackdrift" watch`) {
		t.Fatalf("expected the binary path quoted:\n%s", unit)
	}
}

// One service per machine, not one per scanned directory. The command it runs
// takes no directory: the sweep reads every linked project itself. A unit bound
// to one directory would need a second unit for the next project scanned.
func TestUnitFile_Always_SweepsEveryProjectRatherThanOneDirectory(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "WorkingDirectory=") {
		t.Fatalf("a unit tied to one directory cannot cover the next project:\n%s", unit)
	}

	for _, line := range strings.Split(unit, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "ExecStart=")
		if !found {
			continue
		}
		if strings.TrimSpace(rest) != "/x/stackdrift watch --scheduled" {
			t.Fatalf("the sweep takes no directory argument, got %q", rest)
		}
	}
}

// sd-bus derives the user bus address from XDG_RUNTIME_DIR alone, so ssh, cron
// and su fail even though the socket is sitting there
func TestRuntimeDirEnv_VariableMissingAndDirectoryPresent_IsFilledIn(t *testing.T) {
	if got := runtimeDirEnv("", "/run/user/1000", true); got != "XDG_RUNTIME_DIR=/run/user/1000" {
		t.Fatalf("expected the variable to be supplied, got %q", got)
	}
}

// A caller already pointed at their OWN runtime directory is left alone. There
// is nothing to correct and rewriting it would be noise.
func TestRuntimeDirEnv_AlreadyPointingAtOurOwn_IsLeftAlone(t *testing.T) {
	if got := runtimeDirEnv("/run/user/1000", "/run/user/1000", true); got != "" {
		t.Fatalf("expected nothing added, got %q", got)
	}
}

// The case that took two production servers out. "su ubuntu" from a root shell
// keeps root's XDG_RUNTIME_DIR, because non-login su sets only HOME, SHELL,
// USER and LOGNAME. Every systemctl --user call then aims at uid 0's manager
// while running as uid 1000 and fails, so the install writes its unit files and
// schedules nothing.
//
// This used to be read as "the caller set it deliberately, leave it alone".
// Nobody sets it deliberately to another user's directory; it is inherited, and
// it is always wrong.
func TestRuntimeDirEnv_SetToAnotherUsersDirectory_IsCorrected(t *testing.T) {
	if got := runtimeDirEnv("/run/user/0", "/run/user/1000", true); got != "XDG_RUNTIME_DIR=/run/user/1000" {
		t.Fatalf("a directory belonging to another uid has to be replaced with our own, got %q", got)
	}
}

// Ours has to exist to be worth pointing at, even when theirs is wrong.
func TestRuntimeDirEnv_SetToAnotherUsersDirectoryAndOursIsMissing_IsLeftAlone(t *testing.T) {
	if got := runtimeDirEnv("/run/user/0", "/run/user/1000", false); got != "" {
		t.Fatalf("pointing at a directory that does not exist fixes nothing, got %q", got)
	}
}

// Anything that is not exactly our own path is rewritten to the canonical one.
// Matching loosely and then passing the original through was a hole: a value
// with whitespace around it compares equal after trimming and then fails,
// whitespace and all, in the child process.
func TestRuntimeDirEnv_OurOwnWrittenDifferently_IsCanonicalised(t *testing.T) {
	for _, current := range []string{"/run/user/1000/", " /run/user/1000 ", "/run/user/1000//"} {
		if got := runtimeDirEnv(current, "/run/user/1000", true); got != "XDG_RUNTIME_DIR=/run/user/1000" {
			t.Fatalf("%q should be replaced with the canonical path, got %q", current, got)
		}
	}
}

func TestRuntimeDirEnv_NoRuntimeDirectory_AddsNothing(t *testing.T) {
	if got := runtimeDirEnv("", "/run/user/1000", false); got != "" {
		t.Fatalf("pointing at a directory that does not exist fixes nothing, got %q", got)
	}
}

func TestRuntimeDirEnv_BlankVariable_CountsAsMissing(t *testing.T) {
	if got := runtimeDirEnv("   ", "/run/user/1000", true); got != "XDG_RUNTIME_DIR=/run/user/1000" {
		t.Fatalf("expected a blank value to be treated as unset, got %q", got)
	}
}

// Lingering gives a headless box a user manager so it has to come first
func TestInstall_LingeringIsAttemptedBeforeAnythingNeedsTheUserBus(t *testing.T) {
	calls := recordInstall(t, config.IntervalDaily)

	linger, reload := callIndex(calls, "loginctl enable-linger"), callIndex(calls, "daemon-reload")
	if linger < 0 {
		t.Fatalf("lingering was never attempted, calls were %+v", calls)
	}
	if reload < 0 {
		t.Fatalf("no daemon-reload, calls were %+v", calls)
	}
	if linger > reload {
		t.Fatalf("lingering must come first or a box with no session never reaches it, calls were %+v", calls)
	}
}

// Arming after the reload or systemd enables a unit it has not read
func TestInstall_UnitIsArmedAfterTheReload(t *testing.T) {
	calls := recordInstall(t, config.IntervalDaily)

	if reload, arm := callIndex(calls, "daemon-reload"), callIndex(calls, "enable --now"); reload > arm {
		t.Fatalf("expected the reload before arming, calls were %+v", calls)
	}
}

func recordInstall(t *testing.T, interval string) []string {
	t.Helper()

	if !Supported() {
		t.Skip("systemctl is not on this machine")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USER", "ubuntu")

	var calls []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := Install(Plan{Interval: interval, Exec: "/usr/local/bin/stackdrift"}); err != nil {
		t.Fatal(err)
	}
	return calls
}

func callIndex(calls []string, fragment string) int {
	for i, call := range calls {
		if strings.Contains(call, fragment) {
			return i
		}
	}
	return -1
}

// The one check that talks to real systemd, timers target is read never changed
func TestCommand_NoRuntimeDirInTheEnvironment_StillReachesTheUserBus(t *testing.T) {
	if !Supported() || !runtimeDirExists() {
		t.Skip("no systemctl or no user runtime directory on this machine")
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")

	if err := command("systemctl", "--user", "is-active", "--quiet", "timers.target").Run(); err != nil {
		t.Fatalf("a caller with no session variables must still reach its own bus, got %v", err)
	}
}

// Supplying XDG_RUNTIME_DIR on its own is not enough. sd-bus reads
// DBUS_SESSION_BUS_ADDRESS first and honours it even when it is empty or points
// at a bus this user cannot open, which is what ssh, cron and su leave behind.
// The directory we just supplied is then never consulted and the call fails
// anyway, which is the state every install over ssh was in.
func TestWithoutBusAddress_StaleAddress_IsRemoved(t *testing.T) {
	env := withoutBusAddress([]string{"HOME=/home/vince", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/0/bus", "PATH=/usr/bin"})

	for _, entry := range env {
		if strings.HasPrefix(entry, "DBUS_SESSION_BUS_ADDRESS=") {
			t.Fatalf("the stale address survived: %+v", env)
		}
	}
	if len(env) != 2 {
		t.Fatalf("only the bus address should have gone, got %+v", env)
	}
}

func TestWithoutBusAddress_EmptyAddress_IsRemoved(t *testing.T) {
	env := withoutBusAddress([]string{"DBUS_SESSION_BUS_ADDRESS=", "HOME=/home/vince"})

	if len(env) != 1 || env[0] != "HOME=/home/vince" {
		t.Fatalf("an empty address is honoured by sd-bus too, got %+v", env)
	}
}

func TestWithoutBusAddress_NoAddress_ChangesNothing(t *testing.T) {
	env := withoutBusAddress([]string{"HOME=/home/vince", "PATH=/usr/bin"})

	if len(env) != 2 {
		t.Fatalf("nothing to remove, got %+v", env)
	}
}

// A variable that merely starts with the same letters is a different variable
func TestWithoutBusAddress_SimilarlyNamedVariable_IsKept(t *testing.T) {
	env := withoutBusAddress([]string{"DBUS_SESSION_BUS_ADDRESS_BACKUP=unix:path=/run/user/0/bus"})

	if len(env) != 1 {
		t.Fatalf("only the exact variable goes, got %+v", env)
	}
}

// The bus address is stripped even when the caller's runtime directory is
// already correct. Welding the two together left this exact case broken: on a
// machine whose user-manager private socket is unusable, systemctl falls back to
// the bus address and honours it over the directory, so a stale value still
// killed every call.
func TestCommand_CorrectRuntimeDirButAStaleBusAddress_StillStripsTheAddress(t *testing.T) {
	if !runtimeDirExists() {
		t.Skip("no user runtime directory on this machine")
	}
	t.Setenv("XDG_RUNTIME_DIR", defaultRuntimeDir())
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/run/user/0/bus")

	for _, entry := range command("true").Env {
		if strings.HasPrefix(entry, "DBUS_SESSION_BUS_ADDRESS=") {
			t.Fatal("a bus address belonging to another user must not reach the child")
		}
	}
}

// Nothing is touched when this user has no runtime directory at all, because
// there is no correct value to substitute and the child's own environment is
// the only thing it has to go on.
func TestCommand_NoRuntimeDirectoryForThisUser_LeavesTheEnvironmentAlone(t *testing.T) {
	if runtimeDirExists() {
		t.Skip("this user has a runtime directory, so the branch cannot be reached")
	}

	if command("true").Env != nil {
		t.Fatal("with nothing to correct the child should inherit the environment untouched")
	}
}
