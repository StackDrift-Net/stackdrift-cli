package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHasExecutableMagic_Elf_IsAcceptedOnLinux(t *testing.T) {
	path := writeBytes(t, "bin", []byte("\x7fELF\x02\x01\x01\x00rest"))

	if err := hasExecutableMagic(path, "linux"); err != nil {
		t.Fatalf("expected an ELF accepted on linux, got %v", err)
	}
}

func TestHasExecutableMagic_Pe_IsAcceptedOnWindows(t *testing.T) {
	path := writeBytes(t, "bin.exe", []byte("MZ\x90\x00\x03\x00\x00\x00rest"))

	if err := hasExecutableMagic(path, "windows"); err != nil {
		t.Fatalf("expected a PE accepted on windows, got %v", err)
	}
}

func TestHasExecutableMagic_MachO_IsAcceptedOnDarwin(t *testing.T) {
	for _, magic := range [][]byte{
		{0xcf, 0xfa, 0xed, 0xfe}, // 64 bit little endian
		{0xfe, 0xed, 0xfa, 0xcf}, // 64 bit big endian
		{0xca, 0xfe, 0xba, 0xbe}, // universal
	} {
		path := writeBytes(t, "bin", append(magic, []byte("rest of it")...))
		if err := hasExecutableMagic(path, "darwin"); err != nil {
			t.Fatalf("expected %x accepted on darwin, got %v", magic, err)
		}
	}
}

// The failure this exists for: a proxy, a captive portal or a moved release
// answers 200 with an HTML page, which the old code would have chmod 0755 and
// renamed over the running binary.
func TestHasExecutableMagic_HtmlErrorPage_IsRefused(t *testing.T) {
	path := writeBytes(t, "bin", []byte("<!DOCTYPE html><html><body>Not Found</body></html>"))

	if err := hasExecutableMagic(path, "linux"); err == nil {
		t.Fatal("an HTML page must never be installed as the binary")
	}
}

func TestHasExecutableMagic_BinaryForAnotherPlatform_IsRefused(t *testing.T) {
	path := writeBytes(t, "bin", []byte("MZ\x90\x00rest"))

	if err := hasExecutableMagic(path, "linux"); err == nil {
		t.Fatal("a Windows binary must not be installed on linux")
	}
}

func TestHasExecutableMagic_TruncatedDownload_IsRefused(t *testing.T) {
	path := writeBytes(t, "bin", []byte("\x7fE"))

	if err := hasExecutableMagic(path, "linux"); err == nil {
		t.Fatal("a file too short to carry a header must be refused")
	}
}

func TestHasExecutableMagic_Empty_IsRefused(t *testing.T) {
	path := writeBytes(t, "bin", nil)

	if err := hasExecutableMagic(path, "linux"); err == nil {
		t.Fatal("an empty file must be refused")
	}
}

// A shell script stands in for the downloaded binary. What is under test is
// that the candidate is run and made to name the version it claims to be,
// before it is allowed to replace anything.
func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the trial run is exercised against a shell script")
	}
	path := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTrialRun_ReportsTheExpectedVersion_IsAccepted(t *testing.T) {
	path := script(t, `echo "stackdrift 0.1.45 (server https://stackdrift.net)"`)

	if err := trialRun(path, "v0.1.45"); err != nil {
		t.Fatalf("expected the candidate accepted, got %v", err)
	}
}

// The release feed said one thing and the asset is another. Installing it would
// leave the machine reporting a version nobody published.
func TestTrialRun_ReportsADifferentVersion_IsRefused(t *testing.T) {
	path := script(t, `echo "stackdrift 0.1.2 (server https://stackdrift.net)"`)

	if err := trialRun(path, "v0.1.45"); err == nil {
		t.Fatal("a candidate naming another version must be refused")
	}
}

func TestTrialRun_CannotRunAtAll_IsRefused(t *testing.T) {
	path := writeBytes(t, "candidate", []byte("\x7fELFnot really"))

	if err := trialRun(path, "v0.1.45"); err == nil {
		t.Fatal("a candidate that will not execute must be refused")
	}
}

func TestTrialRun_RunsButSaysNothing_IsRefused(t *testing.T) {
	path := script(t, `exit 0`)

	if err := trialRun(path, "v0.1.45"); err == nil {
		t.Fatal("silence is not confirmation")
	}
}

func TestTrialRun_ExitsNonZero_IsRefused(t *testing.T) {
	path := script(t, `echo "stackdrift 0.1.45"; exit 1`)

	if err := trialRun(path, "v0.1.45"); err == nil {
		t.Fatal("a candidate that fails to run its own version command must be refused")
	}
}

// The download is pinned to the tag that was read from the release feed. Asking
// for "latest" a second time is a second, independent resolution, and the
// release script creates the release before it uploads the assets, so the two
// can genuinely disagree.
func TestDownloadURL_PinsTheTagThatWasInspected(t *testing.T) {
	got := downloadURL("https://github.com", "owner/repo", "v0.1.45", "stackdrift-linux-amd64")
	want := "https://github.com/owner/repo/releases/download/v0.1.45/stackdrift-linux-amd64"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreviousPath_SitsBesideTheBinary(t *testing.T) {
	got := previousPath("/home/u/.local/bin/stackdrift")

	if !strings.HasPrefix(got, "/home/u/.local/bin/") {
		t.Fatalf("the rollback copy has to be on the same filesystem to be renamed, got %q", got)
	}
	if got == "/home/u/.local/bin/stackdrift" {
		t.Fatal("the rollback copy must not be the binary itself")
	}
}
