package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/StackDrift-Net/stackdrift-cli/internal/commands"
)

// Set on the child so a build that is still refused after updating reports the
// refusal instead of updating and re-running for ever.
const upgradedMarker = "STACKDRIFT_UPGRADED"

// upgradeAndRerun replaces this build with the current release and runs the
// same command again, returning the exit code to finish on.
//
// The update comes from the release feed rather than the API, which is what
// makes a refusal recoverable: the build being turned away can still reach the
// thing that replaces it.
func upgradeAndRerun(args []string, stdout, stderr io.Writer, refusal error) int {
	fmt.Fprintln(stdout, "This CLI is behind the version the server requires. Updating...")

	if err := commands.Update(version, nil); err != nil {
		fmt.Fprintln(stderr, "error: could not update: "+err.Error())
		fmt.Fprintln(stderr, "error: "+refusal.Error())
		return 1
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "error: updated, but could not find this program to re-run it: "+err.Error())
		return 1
	}

	fmt.Fprintln(stdout)
	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), upgradedMarker+"=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(stderr, "error: updated, but could not re-run the command: "+err.Error())
		return 1
	}

	return 0
}

// alreadyUpgraded reports whether this process is the re-run, in which case a
// second refusal is final.
func alreadyUpgraded() bool {
	return os.Getenv(upgradedMarker) != ""
}
