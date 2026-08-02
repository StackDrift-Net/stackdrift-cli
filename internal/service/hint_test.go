package service

import (
	"errors"
	"strings"
	"testing"
)

// The exact text the box answers with, as run formats it
const busFailure = "systemctl --user daemon-reload: Failed to connect to bus: No medium found"

func TestInstallHint_NoSessionBus_SaysNotToUseSudo(t *testing.T) {
	hint := strings.Join(InstallHint(errors.New(busFailure), "ubuntu"), "\n")

	if !strings.Contains(hint, "sudo loginctl enable-linger ubuntu") {
		t.Fatalf("the remedy must be copy-pasteable for this user, got %q", hint)
	}
	if !strings.Contains(strings.ToLower(hint), "not run this with sudo") {
		t.Fatalf("running the command itself under sudo is the other cause, got %q", hint)
	}
}

// qc-dev runs sshd with UsePAM no so no session is ever registered
func TestInstallHint_NoSessionBus_NamesLingeringAsTheServerRemedy(t *testing.T) {
	hint := strings.Join(InstallHint(errors.New(busFailure), "ubuntu"), "\n")

	if !strings.Contains(hint, "enable-linger") {
		t.Fatalf("a box that issues no login sessions needs lingering, got %q", hint)
	}
}

func TestInstallHint_UnknownUser_StillReadsAsACommand(t *testing.T) {
	hint := strings.Join(InstallHint(errors.New(busFailure), ""), "\n")

	if !strings.Contains(hint, "enable-linger <user>") {
		t.Fatalf("expected a placeholder rather than a broken command, got %q", hint)
	}
}

func TestInstallHint_UnrecognisedFailure_SaysNothing(t *testing.T) {
	if hint := InstallHint(errors.New("systemctl --user enable --now: no such file"), "ubuntu"); hint != nil {
		t.Fatalf("a guess is worse than silence, got %+v", hint)
	}
}

func TestInstallHint_NoError_SaysNothing(t *testing.T) {
	if hint := InstallHint(nil, "ubuntu"); hint != nil {
		t.Fatalf("expected nothing, got %+v", hint)
	}
}
