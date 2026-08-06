package service

import (
	"fmt"
	"strings"
)

// The service belongs to the account that installs it, because the credentials
// the sweep needs live in that account's own config directory. So a machine can
// end up with one watcher per account and nothing in either account's view of
// the world mentions the other.
//
// Looking for the others is best effort by nature. An account can only see
// another account's files where the permissions allow it, which is usually true
// for root looking down and never true the other way. The pure parts live here
// with no build constraint so they are tested on every platform.

// Installation is another account's watcher, found on this machine.
type Installation struct {
	Account string
	Detail  string
}

type accountHome struct {
	Account string
	Home    string
}

// Homes that cannot hold a user unit. Every system account points at one of
// these, and checking each one would be a stat per account on every install.
var homelessPaths = map[string]bool{
	"":             true,
	"/":            true,
	"/bin":         true,
	"/dev/null":    true,
	"/nonexistent": true,
	"/proc":        true,
	"/run":         true,
	"/sbin":        true,
	"/srv":         true,
	"/usr/sbin":    true,
	"/var/run":     true,
}

// homesFromPasswd reads the accounts worth checking out of an /etc/passwd body.
// skipHome is the caller's own home, which has already answered for itself.
func homesFromPasswd(body, skipHome string) []accountHome {
	var accounts []accountHome
	seen := map[string]bool{}

	for _, line := range strings.Split(body, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) < 6 {
			continue
		}

		name, home := fields[0], fields[5]
		if name == "" || homelessPaths[home] || home == skipHome || seen[home] {
			continue
		}

		seen[home] = true
		accounts = append(accounts, accountHome{Account: name, Home: home})
	}
	return accounts
}

// runAsUserFromTask reads the owner out of a verbose schtasks query. Windows
// registers one task for the machine rather than one per account, so a second
// account does not add a watcher, it takes the existing one over.
func runAsUserFromTask(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Run As User:"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// OtherAccountLines is what the user is told. It says what was found and when
// it matters, because a second watcher is not wrong in itself. Each one only
// ever sweeps the projects its own account scanned.
func OtherAccountLines(found []Installation, current string) []string {
	if len(found) == 0 {
		return nil
	}

	heading := "Another account on this machine already has a StackDrift watcher."
	if len(found) > 1 {
		heading = "Other accounts on this machine already have StackDrift watchers."
	}

	lines := []string{heading}
	for _, installation := range found {
		lines = append(lines, fmt.Sprintf("  %s: %s", installation.Account, installation.Detail))
	}

	return append(lines,
		"",
		"A watcher only sweeps the systems its own account scanned, so this is only",
		"a problem if both accounts scanned the same directory. Note that this check",
		"can only see what "+current+" is allowed to read.")
}
