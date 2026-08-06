package service

import (
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// The plist and schtasks halves are deliberately not build constrained, so they
// run on every platform. The systemd half needs unitFile, which is Linux only,
// and lives in installedexec_linux_test.go.

func TestExecFromPlist_ReadsThePathTheAgentStarts(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/usr/local/bin/stackdrift"})

	if got := execFromPlist(body); got != "/usr/local/bin/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromPlist_EscapedPath_ComesBackAsItWent(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/opt/a&b/stackdrift"})

	if got := execFromPlist(body); got != "/opt/a&b/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromPlist_NoArguments_IsEmpty(t *testing.T) {
	if got := execFromPlist("<plist><dict></dict></plist>"); got != "" {
		t.Fatalf("expected nothing, got %q", got)
	}
}

func TestExecFromTask_ReadsThePathTheTaskRuns(t *testing.T) {
	output := "Folder: \\\r\nTaskName:      \\StackDrift Watch\r\n" +
		"Task To Run:   C:\\Users\\u\\stackdrift.exe watch --scheduled\r\nStatus:        Ready\r\n"

	if got := execFromTask(output); got != `C:\Users\u\stackdrift.exe` {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromTask_QuotedPath_ComesBackUnquoted(t *testing.T) {
	output := "Task To Run:   \"C:\\Program Files\\StackDrift\\stackdrift.exe\" watch --scheduled\r\n"

	if got := execFromTask(output); got != `C:\Program Files\StackDrift\stackdrift.exe` {
		t.Fatalf("got %q", got)
	}
}

// The non-verbose query does not carry the field at all, so nothing may be
// invented from it.
func TestExecFromTask_FieldAbsent_IsEmpty(t *testing.T) {
	if got := execFromTask("TaskName: \\StackDrift Watch\r\nStatus: Ready\r\n"); got != "" {
		t.Fatalf("expected nothing, got %q", got)
	}
}

func TestExecFromPlist_PathHoldingATab_RoundTrips(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/opt/a\tb/stackdrift"})

	if got := execFromPlist(body); got != "/opt/a\tb/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromPlist_PathHoldingAnEncodedAmpersand_IsNotDecodedTwice(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/opt/a&lt;b/stackdrift"})

	if got := execFromPlist(body); got != "/opt/a&lt;b/stackdrift" {
		t.Fatalf("got %q", got)
	}
}
