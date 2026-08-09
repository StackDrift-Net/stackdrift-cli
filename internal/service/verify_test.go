package service

import (
	"errors"
	"strings"
	"testing"
)

var errNoStatus = errors.New("cannot reach the scheduler")

func TestVerifyInstalled_SchedulerHoldsIt_Passes(t *testing.T) {
	state := State{Installed: true, Enabled: true, Running: true}

	if err := verifyInstalled(state, nil); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

// The failure this whole check exists for. The unit files were written, the
// enable did not take, and the old code called that a successful install.
func TestVerifyInstalled_UnitWrittenButNotEnabled_Fails(t *testing.T) {
	state := State{Installed: true, Enabled: false, Running: false}

	err := verifyInstalled(state, nil)
	if err == nil {
		t.Fatal("want an error, got none")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("want the error to say it is not enabled, got %q", err)
	}
}

func TestVerifyInstalled_EnabledButTheSchedulerIsNotHoldingIt_Fails(t *testing.T) {
	state := State{Installed: true, Enabled: true, Running: false}

	err := verifyInstalled(state, nil)
	if err == nil {
		t.Fatal("want an error, got none")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("want the error to say it is not running, got %q", err)
	}
}

func TestVerifyInstalled_NothingWasWritten_Fails(t *testing.T) {
	err := verifyInstalled(State{}, nil)

	if err == nil {
		t.Fatal("want an error, got none")
	}
	if !strings.Contains(err.Error(), "was not created") {
		t.Fatalf("want the error to name the missing unit, got %q", err)
	}
}

// A platform that cannot answer must not be read as a failed install, or every
// install on it would be rolled back over a question nobody could ask.
func TestVerifyInstalled_StatusCouldNotBeRead_Passes(t *testing.T) {
	if err := verifyInstalled(State{}, errNoStatus); err != nil {
		t.Fatalf("want no error when the state is unknown, got %v", err)
	}
}

func TestInstallVerified_InstallFails_DoesNotReportTheStateInstead(t *testing.T) {
	restore := swapInstallProbes(
		func(Plan) error { return errNoStatus },
		func() (State, error) { return State{Installed: true, Enabled: true, Running: true}, nil },
	)
	defer restore()

	if err := InstallVerified(Plan{Interval: "daily", Exec: "/bin/true"}); err == nil {
		t.Fatal("want the install error to survive, got none")
	}
}

func TestInstallVerified_InstallSucceedsButNothingIsScheduled_Fails(t *testing.T) {
	restore := swapInstallProbes(
		func(Plan) error { return nil },
		func() (State, error) { return State{Installed: true}, nil },
	)
	defer restore()

	if err := InstallVerified(Plan{Interval: "daily", Exec: "/bin/true"}); err == nil {
		t.Fatal("want an error when the install did not take, got none")
	}
}

func TestInstallVerified_SchedulerHoldsIt_Passes(t *testing.T) {
	restore := swapInstallProbes(
		func(Plan) error { return nil },
		func() (State, error) { return State{Installed: true, Enabled: true, Running: true}, nil },
	)
	defer restore()

	if err := InstallVerified(Plan{Interval: "daily", Exec: "/bin/true"}); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func swapInstallProbes(install func(Plan) error, status func() (State, error)) func() {
	previousInstall, previousStatus := installFunc, statusFunc
	installFunc, statusFunc = install, status
	return func() { installFunc, statusFunc = previousInstall, previousStatus }
}
