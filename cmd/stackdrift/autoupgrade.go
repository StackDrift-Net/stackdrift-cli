package main

import (
	"fmt"
	"io"

	"github.com/StackDrift-Net/stackdrift-cli/internal/commands"
)

// upgradeAndRerun replaces this build with the current release and runs the
// same command again, returning the exit code to finish on.
//
// The update comes from the release feed rather than the API, which is what
// makes a refusal recoverable: the build being turned away can still reach the
// thing that replaces it.
func upgradeAndRerun(args []string, stdout, stderr io.Writer, refusal error) int {
	fmt.Fprintln(stdout, "This CLI is behind the version the server requires. Updating...")

	installed, err := commands.Update(version, nil)
	if err != nil {
		fmt.Fprintln(stderr, "error: could not update: "+err.Error())
		fmt.Fprintln(stderr, "error: "+refusal.Error())
		return 1
	}

	fmt.Fprintln(stdout)
	code, err := commands.RerunSelf(installed, args, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "error: updated, but could not re-run the command: "+err.Error())
		return 1
	}
	return code
}

// alreadyUpgraded reports whether this process is the re-run, in which case a
// second refusal is final.
func alreadyUpgraded() bool {
	return commands.AlreadyUpgraded()
}
