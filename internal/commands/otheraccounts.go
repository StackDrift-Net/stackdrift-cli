package commands

import (
	"os"
	"os/user"

	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// Swapped in tests, which must not depend on who else has an account on the
// machine running them
var installedElsewhere = service.InstalledElsewhere

// confirmNoDoubleUp reports whether the install should go ahead when another
// account on this machine already has a watcher.
//
// It warns rather than refuses. The check can only see what this account is
// allowed to read, so refusing would block a legitimate install for a reason
// the user cannot see, and a second watcher is not wrong in itself.
//
// A run with nobody to ask proceeds. A scripted install must not stop on a
// question, and there is nothing to double up on that the operator did not
// already ask for.
func confirmNoDoubleUp() bool {
	account := currentAccount()
	lines := service.OtherAccountLines(installedElsewhere(), account)
	if len(lines) == 0 {
		return true
	}

	ui.Println()
	for _, line := range lines {
		ui.Println(line)
	}
	ui.Println()

	if !ui.Interactive() {
		return true
	}
	return ui.Confirm("Install one for "+account+" as well?", false)
}

func currentAccount() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	return "this account"
}
