package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// UpgradedMarker is set on the child so a build that is still behind after
// updating reports that, instead of updating and re-running for ever.
const UpgradedMarker = "STACKDRIFT_UPGRADED"

// AlreadyUpgraded reports whether this process is the re-run, in which case
// there is nothing further to gain from updating again.
func AlreadyUpgraded() bool {
	return os.Getenv(UpgradedMarker) != ""
}

// RerunSelf runs the same command again with the binary now installed at self,
// and returns the child's exit code.
//
// The current process cannot pick up the new code itself: the kernel is still
// running the image it loaded, so the only way to run what was downloaded is to
// exec it.
//
// The path is a parameter rather than something this works out, because it
// cannot be worked out here. Moving the running image aside makes
// /proc/self/exe, and so os.Executable, resolve to the displaced copy, so
// asking would re-run the build that was just replaced.
func RerunSelf(self string, args []string, stdout, stderr io.Writer) (int, error) {
	if self == "" {
		return 1, errors.New("no path was given for the program to re-run")
	}

	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), UpgradedMarker+"=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// ExitCodeError carries a code decided by something other than the usual
// reading of a failure, so a command that has handed its work to a child
// process finishes on the child's result rather than a flat 1.
// Err is what the child was finishing on, kept so the reading of this error
// can be told apart from the reading of what it wraps. Without it a caller
// that judged the wrapped failure first would look identical to one that
// judged this first.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("exit status %d: %s", e.Code, e.Err.Error())
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error { return e.Err }

// ExitCode reports the code an error carries, when it carries one.
func ExitCode(err error) (int, bool) {
	var coded *ExitCodeError
	if errors.As(err, &coded) {
		return coded.Code, true
	}
	return 0, false
}
