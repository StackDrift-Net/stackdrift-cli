package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/commands"
)

// A scheduled watch that updated itself hands the work to the new binary and
// finishes on whatever that reported. Flattening it to 1 would tell a scheduler
// the run failed when it succeeded.
func TestRun_CommandFinishedByAChildProcess_UsesTheChildsCode(t *testing.T) {
	code, _ := runWith(t, &commands.ExitCodeError{Code: exitPlanLapsed})

	if code != exitPlanLapsed {
		t.Fatalf("expected the child's code %d, got %d", exitPlanLapsed, code)
	}
}

func TestRun_ChildSucceeded_ExitsZero(t *testing.T) {
	code, _ := runWith(t, &commands.ExitCodeError{Code: 0})

	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

// The child has already been through every judgement below this one, including
// the 426 upgrade path. A refusal it finished on has to be read as its exit
// code, not taken as a reason to update and re-run a second time.
//
// The wrapped refusal is what makes the ordering observable: with the readings
// the other way round this returns 1 from the upgrade path instead of 7.
func TestRun_ChildFinishedOnARefusal_IsNotUpgradedAgain(t *testing.T) {
	// The marker is deliberately NOT set, because with it there is no upgrade
	// path to get the ordering wrong against. The feed is pointed at a closed
	// port instead, so a wrong reading fails fast rather than reaching the real
	// releases and replacing the test binary, which is what it did once.
	t.Setenv("STACKDRIFT_UPDATE_API", "http://127.0.0.1:1")
	t.Setenv("STACKDRIFT_UPDATE_DOWNLOAD", "http://127.0.0.1:1")

	var calls []string
	code := run(
		[]string{"scan"},
		stubRegistry(&calls, &commands.ExitCodeError{
			Code: 7,
			Err:  &api.Error{Status: http.StatusUpgradeRequired, Message: "CLI update required"},
		}),
		&bytes.Buffer{}, &bytes.Buffer{},
	)

	if code != 7 {
		t.Fatalf("expected the child's code carried through, got %d", code)
	}
}

// Nothing else changes: an ordinary API failure is still read the way it was.
func TestRun_OrdinaryFailure_StillExitsOne(t *testing.T) {
	code, _ := runWith(t, &api.Error{Status: http.StatusInternalServerError, Message: "boom"})

	if code != exitFailure {
		t.Fatalf("expected %d, got %d", exitFailure, code)
	}
}
