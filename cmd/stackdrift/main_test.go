package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func stubRegistry(calls *[]string, err error) map[string]command {
	record := func(name string) func([]string) error {
		return func(args []string) error {
			*calls = append(*calls, name+" "+strings.Join(args, " "))
			return err
		}
	}
	return map[string]command{
		"scan":  {name: "scan", run: record("scan"), help: "detect technologies"},
		"check": {name: "check", run: record("check"), help: "report CVE status"},
	}
}

func TestRun_KnownCommand_SucceedsAndForwardsArguments(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer

	code := run([]string{"scan", "--yes"}, stubRegistry(&calls, nil), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(calls) != 1 || calls[0] != "scan --yes" {
		t.Fatalf("expected the flags forwarded to the command, got %v", calls)
	}
}

func TestRun_CommandFails_ExitsNonZeroAndReportsOnStderr(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer

	code := run([]string{"check"}, stubRegistry(&calls, errors.New("2 technology CVEs found")), &stdout, &stderr)

	// This is the CI gate: a failing check has to exit non-zero.
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "2 technology CVEs found") {
		t.Fatalf("expected the reason on stderr, got %q", stderr.String())
	}
}

func TestRun_NoArguments_ShowsUsageAndFails(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer

	code := run(nil, stubRegistry(&calls, nil), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: stackdrift") {
		t.Fatalf("expected usage on stdout, got %q", stdout.String())
	}
	if len(calls) != 0 {
		t.Fatalf("expected no command to run, got %v", calls)
	}
}

func TestRun_UnknownCommand_NamesItAndFails(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer

	code := run([]string{"scna"}, stubRegistry(&calls, nil), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: scna") {
		t.Fatalf("expected the typo echoed back, got %q", stderr.String())
	}
	if len(calls) != 0 {
		t.Fatalf("expected no command to run, got %v", calls)
	}
}

func TestRun_HelpFlags_ShowUsageAndSucceed(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		var calls []string
		var stdout, stderr bytes.Buffer

		code := run([]string{arg}, stubRegistry(&calls, nil), &stdout, &stderr)

		if code != 0 {
			t.Fatalf("expected %q to exit 0, got %d", arg, code)
		}
		if !strings.Contains(stdout.String(), "Usage: stackdrift") {
			t.Fatalf("expected usage for %q, got %q", arg, stdout.String())
		}
	}
}

func TestRegistry_DispatchesEveryDeclaredCommand(t *testing.T) {
	actual := registry()

	for _, c := range commandList() {
		if _, ok := actual[c.name]; !ok {
			t.Fatalf("%q is declared but cannot be dispatched", c.name)
		}
		if c.run == nil {
			t.Fatalf("%q has nothing to run", c.name)
		}
	}
	if len(actual) != len(commandList()) {
		t.Fatalf("expected one registry entry per command, got %d for %d", len(actual), len(commandList()))
	}
}

func TestUsage_DescribesEveryVisibleCommand(t *testing.T) {
	var out bytes.Buffer
	usage(&out)

	for _, c := range commandList() {
		if c.hidden {
			continue
		}
		if !strings.Contains(out.String(), c.name) {
			t.Fatalf("expected %q in the usage text", c.name)
		}
		if !strings.Contains(out.String(), c.help) {
			t.Fatalf("expected the description for %q in the usage text", c.name)
		}
	}
}

// The override stays, the advertisement does not. It exists to point the CLI
// at staging, which is not something to put in front of users.
func TestUsage_DoesNotAdvertiseTheServerOverride(t *testing.T) {
	var out bytes.Buffer
	usage(&out)

	if strings.Contains(out.String(), "STACKDRIFT_URL") {
		t.Fatalf("expected the server override kept out of the usage text, got %q", out.String())
	}
}

func TestUsage_HidesInternalCommands(t *testing.T) {
	var out bytes.Buffer
	usage(&out)

	// __complete exists for the shell, not for a person reading the help.
	if strings.Contains(out.String(), "__complete") {
		t.Fatalf("expected the completion helper hidden from usage, got %q", out.String())
	}
}

func TestCompletionInfo_ExcludesTheHiddenHelper(t *testing.T) {
	for _, info := range completionInfo() {
		if info.Name == "__complete" {
			t.Fatal("the completion helper must not suggest itself")
		}
	}
}

func TestCompletionInfo_CarriesTheOptionsOfEachCommand(t *testing.T) {
	options := map[string][]string{}
	for _, info := range completionInfo() {
		for _, option := range info.Options {
			options[info.Name] = append(options[info.Name], option.Name)
		}
	}

	if !strings.Contains(strings.Join(options["scan"], " "), "--yes") {
		t.Fatalf("expected scan to offer --yes, got %v", options["scan"])
	}
	if !strings.Contains(strings.Join(options["completion"], " "), "bash") {
		t.Fatalf("expected completion to offer the supported shells, got %v", options["completion"])
	}
}
