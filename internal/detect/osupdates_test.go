package detect

import (
	"encoding/base64"
	"runtime"
	"strings"
	"testing"
)

func TestParseUpdateCounts_AFullAnswer_ReadsBothNumbers(t *testing.T) {
	status, ok := parseUpdateCounts("searched=2026-08-05T09:12:33.0000000Z\npending=12\nsecurity=3\n")

	if !ok {
		t.Fatal("expected a complete answer to be usable")
	}
	if status.Pending != 12 || status.Security != 3 {
		t.Fatalf("got %d pending and %d security", status.Pending, status.Security)
	}
}

// PowerShell ends every line with a carriage return, so the values arrive with
// one glued to them.
func TestParseUpdateCounts_WindowsLineEndings_AreStripped(t *testing.T) {
	status, ok := parseUpdateCounts("searched=2026-08-05T09:12:33Z\r\npending=4\r\nsecurity=1\r\n")

	if !ok || status.Pending != 4 || status.Security != 1 {
		t.Fatalf("got %+v ok=%v", status, ok)
	}
}

// Windows answers out of its own cache, so a machine whose update search has
// never come back reports nothing waiting when the truth is nobody has looked.
func TestParseUpdateCounts_NoSearchHasEverSucceeded_RefusesToCallItClean(t *testing.T) {
	if _, ok := parseUpdateCounts("searched=\npending=0\nsecurity=0\n"); ok {
		t.Fatal("a cache nobody has ever filled must not read as nothing waiting")
	}
}

// The same machine reporting updates is telling the truth whether or not it has
// a search date, because it could not have named them otherwise.
func TestParseUpdateCounts_NoSearchDateButUpdatesFound_IsStillTrusted(t *testing.T) {
	status, ok := parseUpdateCounts("searched=\npending=7\nsecurity=2\n")

	if !ok || status.Pending != 7 {
		t.Fatalf("got %+v ok=%v", status, ok)
	}
}

func TestParseUpdateCounts_SearchSucceededAndNothingIsWaiting_IsAnAnswer(t *testing.T) {
	status, ok := parseUpdateCounts("searched=2026-08-05T09:12:33Z\npending=0\nsecurity=0\n")

	if !ok || status.Pending != 0 {
		t.Fatalf("got %+v ok=%v", status, ok)
	}
}

func TestParseUpdateCounts_NoPendingLine_IsNoAnswer(t *testing.T) {
	if _, ok := parseUpdateCounts("searched=2026-08-05T09:12:33Z\nsecurity=3\n"); ok {
		t.Fatal("expected no answer without a total")
	}
}

func TestParseUpdateCounts_Empty_IsNoAnswer(t *testing.T) {
	if _, ok := parseUpdateCounts(""); ok {
		t.Fatal("expected an empty probe to answer nothing")
	}
}

// A machine that refused the query prints its complaint on the same stream, and
// none of it parses.
func TestParseUpdateCounts_AnErrorMessage_IsNoAnswer(t *testing.T) {
	if _, ok := parseUpdateCounts("Exception calling Search: 0x8024402C"); ok {
		t.Fatal("expected a failed query to answer nothing")
	}
}

func TestParseUpdateCounts_NegativeTotal_IsNoAnswer(t *testing.T) {
	if _, ok := parseUpdateCounts("searched=2026-08-05T09:12:33Z\npending=-1\n"); ok {
		t.Fatal("expected a negative total to be refused")
	}
}

// The total is the number worth having, so a security count that will not read
// falls back to none rather than throwing the answer away.
func TestParseUpdateCounts_UnreadableSecurityCount_KeepsTheTotal(t *testing.T) {
	status, ok := parseUpdateCounts("searched=2026-08-05T09:12:33Z\npending=9\nsecurity=\n")

	if !ok || status.Pending != 9 || status.Security != 0 {
		t.Fatalf("got %+v ok=%v", status, ok)
	}
}

// Security updates are a subset of the total, so more of them than there are
// updates describes something impossible.
func TestParseUpdateCounts_MoreSecurityThanTotal_IsHeldToTheTotal(t *testing.T) {
	status, _ := parseUpdateCounts("searched=2026-08-05T09:12:33Z\npending=2\nsecurity=40\n")

	if status.Security != 2 {
		t.Fatalf("expected the security count held to 2, got %d", status.Security)
	}
}

func TestParseUpdateCounts_UnknownKeys_AreIgnored(t *testing.T) {
	status, ok := parseUpdateCounts("noise=yes\nsearched=2026-08-05T09:12:33Z\npending=5\nreboot=true\n")

	if !ok || status.Pending != 5 {
		t.Fatalf("got %+v ok=%v", status, ok)
	}
}

// PowerShell reads EncodedCommand as base64 UTF-16, little end first, so every
// ASCII character has to arrive followed by a zero byte.
func TestEncodeCommand_IsBase64Utf16LittleEndian(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(encodeCommand("Hi"))
	if err != nil {
		t.Fatal(err)
	}

	if string(decoded) != "H\x00i\x00" {
		t.Fatalf("got %q", decoded)
	}
}

// The script is passed encoded precisely so quotes and dollar signs never reach
// a command line to be re-parsed.
func TestEncodeCommand_QuotesAndVariables_Survive(t *testing.T) {
	script := `$x = 'a b'; Write-Output "x=$x"`
	decoded, _ := base64.StdEncoding.DecodeString(encodeCommand(script))

	if strings.ReplaceAll(string(decoded), "\x00", "") != script {
		t.Fatalf("got %q", decoded)
	}
}

// Off Windows there is nobody to ask, and that has to stay distinct from a
// machine answering that it is up to date.
func TestPendingUpdates_WhereThereIsNoWindowsUpdate_AnswersNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is a real Windows Update to talk to here")
	}

	if _, ok := PendingUpdates(nil); ok {
		t.Fatal("expected no answer on a platform with no Windows Update")
	}
}

// The caller uses this to say what it is waiting on, so announcing a wait that
// is never going to happen would print a line and then nothing.
func TestPendingUpdates_NothingToAsk_AnnouncesNoWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is a real Windows Update to talk to here")
	}

	announced := false
	PendingUpdates(func() { announced = true })

	if announced {
		t.Fatal("expected no wait announced where there is nobody to ask")
	}
}
