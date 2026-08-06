package service

import (
	"os"
	"path/filepath"
)

// InstalledElsewhere lists the other accounts on this machine that already have
// a watcher, as far as the caller is allowed to see.
//
// Accounts come from /etc/passwd, which is world readable. A directory service
// can hold accounts that are not in it, so a machine on LDAP or SSSD may have
// installs this never sees, which is why the caller says the check is partial.
func InstalledElsewhere() []Installation {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	body, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}

	var found []Installation
	for _, account := range homesFromPasswd(string(body), home) {
		unit := filepath.Join(account.Home, ".config", "systemd", "user", Name+".service")
		if _, err := os.Stat(unit); err != nil {
			continue
		}
		found = append(found, Installation{Account: account.Account, Detail: unit})
	}
	return found
}
