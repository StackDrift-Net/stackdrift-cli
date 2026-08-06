package service

import (
	"os"
	"path/filepath"
)

// InstalledElsewhere lists the other accounts on this machine that already have
// a watcher, as far as the caller is allowed to see.
//
// The accounts come from /Users rather than /etc/passwd, which on macOS holds
// only the system accounts; the real directory is behind dscl and reading it
// means running a command, which this is not worth.
func InstalledElsewhere() []Installation {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	entries, err := os.ReadDir("/Users")
	if err != nil {
		return nil
	}

	var found []Installation
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "Shared" {
			continue
		}

		candidate := filepath.Join("/Users", entry.Name())
		if candidate == home {
			continue
		}

		plist := filepath.Join(candidate, "Library", "LaunchAgents", label+".plist")
		if _, err := os.Stat(plist); err != nil {
			continue
		}
		found = append(found, Installation{Account: entry.Name(), Detail: plist})
	}
	return found
}
