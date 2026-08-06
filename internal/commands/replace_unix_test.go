//go:build !windows

package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The replace has to be exercised by a process that is actually running the
// binary being replaced, because that is the whole difficulty: moving the
// running image is what makes os.Executable resolve somewhere else afterwards.
// So the test binary copies itself, and the copy does the work.
const replaceHelperDir = "STACKDRIFT_TEST_REPLACE_DIR"

func helperCopy(t *testing.T, dir string) string {
	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Skip("no path to this test binary")
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Skip("cannot read this test binary")
	}

	path := filepath.Join(dir, "stackdrift")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runHelper(t *testing.T, exe, dir, testName string) (string, error) {
	t.Helper()

	cmd := exec.Command(exe, "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), replaceHelperDir+"="+dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The path replaceRunning reports must be where the new binary landed. It
// cannot be worked out afterwards: moving the running image aside makes
// /proc/self/exe resolve to the displaced copy, so a caller that asked again
// would re-run the build it had just replaced.
func TestReplaceRunning_Always_ReportsWhereTheNewBinaryLanded(t *testing.T) {
	if dir := os.Getenv(replaceHelperDir); dir != "" {
		installed, err := replaceRunning(filepath.Join(dir, "candidate"))
		if err != nil {
			os.Stdout.WriteString("HELPER-ERR " + err.Error() + "\n")
			os.Exit(1)
		}
		os.Stdout.WriteString("HELPER-OK[" + installed + "]\n")
		os.Exit(0)
	}

	dir := t.TempDir()
	exe := helperCopy(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "candidate"), []byte("the new build"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runHelper(t, exe, dir, "TestReplaceRunning_Always_ReportsWhereTheNewBinaryLanded")
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	// Bracketed and matched whole, because previousPath appends a suffix, so a
	// Contains on the bare path would hold for the displaced copy as well and
	// the regression this guards would walk straight back in.
	if !strings.Contains(out, "HELPER-OK["+exe+"]") {
		t.Fatalf("expected the installed path to be %q, got:\n%s", exe, out)
	}
}

func TestReplaceRunning_Always_LeavesTheNewBinaryInPlace(t *testing.T) {
	if dir := os.Getenv(replaceHelperDir); dir != "" {
		if _, err := replaceRunning(filepath.Join(dir, "candidate")); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	dir := t.TempDir()
	exe := helperCopy(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "candidate"), []byte("the new build"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runHelper(t, exe, dir, "TestReplaceRunning_Always_LeavesTheNewBinaryInPlace"); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}

	installed, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "the new build" {
		t.Fatalf("expected the new bytes at %s", exe)
	}
	if info, err := os.Stat(exe); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the installed binary has to be executable, got %v %v", info, err)
	}
}

// Unattended there is nobody to fetch another copy, and the program that would
// do it is the one that was overwritten, so the displaced build is the only way
// back.
func TestReplaceRunning_Always_KeepsTheDisplacedBinary(t *testing.T) {
	if dir := os.Getenv(replaceHelperDir); dir != "" {
		if _, err := replaceRunning(filepath.Join(dir, "candidate")); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	dir := t.TempDir()
	exe := helperCopy(t, dir)
	original, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "candidate"), []byte("the new build"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runHelper(t, exe, dir, "TestReplaceRunning_Always_KeepsTheDisplacedBinary"); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}

	kept, err := os.ReadFile(previousPath(exe))
	if err != nil {
		t.Fatalf("expected the displaced build kept at %s: %v", previousPath(exe), err)
	}
	if string(kept) != string(original) {
		t.Fatal("the kept copy is not the build that was displaced")
	}
}

// The replacement failing halfway is the case that could leave a machine with
// no binary where its name is supposed to be. A candidate on another filesystem
// makes the second rename fail for real rather than by injection.
func TestReplaceRunning_ReplacementCannotBeMovedIntoPlace_PutsTheOldOneBack(t *testing.T) {
	if dir := os.Getenv(replaceHelperDir); dir != "" {
		if _, err := replaceRunning(os.Getenv("STACKDRIFT_TEST_CANDIDATE")); err == nil {
			os.Stdout.WriteString("HELPER-UNEXPECTED-SUCCESS\n")
		}
		os.Exit(0)
	}

	if _, err := os.Stat("/dev/shm"); err != nil {
		t.Skip("no second filesystem to rename across")
	}

	dir := t.TempDir()
	exe := helperCopy(t, dir)
	original, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}

	candidate, err := os.CreateTemp("/dev/shm", "stackdrift-candidate-*")
	if err != nil {
		t.Skip("cannot write to the second filesystem")
	}
	candidate.WriteString("the new build")
	candidate.Close()
	defer os.Remove(candidate.Name())

	cmd := exec.Command(exe, "-test.run=^TestReplaceRunning_ReplacementCannotBeMovedIntoPlace_PutsTheOldOneBack$")
	cmd.Env = append(os.Environ(), replaceHelperDir+"="+dir, "STACKDRIFT_TEST_CANDIDATE="+candidate.Name())
	out, _ := cmd.CombinedOutput()

	if strings.Contains(string(out), "HELPER-UNEXPECTED-SUCCESS") {
		t.Skip("the two paths turned out to be on one filesystem, so nothing failed")
	}

	restored, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("the binary must still be where its name says: %v", err)
	}
	if string(restored) != string(original) {
		t.Fatal("expected the original build put back after a failed replace")
	}
}
