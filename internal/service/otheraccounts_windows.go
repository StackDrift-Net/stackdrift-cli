package service

import (
	"os"
	"os/exec"
	"strings"
)

// InstalledElsewhere reports a watcher registered by another account.
//
// Windows differs from the other two platforms. The task is registered once for
// the machine rather than once per account, so a second account does not add a
// watcher, it takes over the one that is there and runs it as itself. The owner
// is read out of the verbose query, which is the only thing that says whose it
// currently is.
func InstalledElsewhere() []Installation {
	output, err := exec.Command("schtasks.exe", "/query", "/tn", taskName, "/v", "/fo", "list").CombinedOutput()
	if err != nil {
		return nil
	}

	owner := runAsUserFromTask(string(output))
	if owner == "" || sameAccount(owner, os.Getenv("USERNAME")) {
		return nil
	}
	return []Installation{{Account: owner, Detail: "scheduled task " + taskName + ", which installing again would take over"}}
}

// Windows reports the owner in whatever case the account was created with, and
// sometimes with the domain in front of it.
func sameAccount(owner, current string) bool {
	if current == "" {
		return false
	}
	if slash := strings.LastIndexAny(owner, `\/`); slash >= 0 {
		owner = owner[slash+1:]
	}
	return strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(current))
}
