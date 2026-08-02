package service

import "strings"

// Known causes only, a guess sends the user changing things that were never wrong
func InstallHint(err error, user string) []string {
	if err == nil {
		return nil
	}

	// Install already tried the env fix and lingering, what is left needs root
	if strings.Contains(strings.ToLower(err.Error()), "failed to connect to bus") {
		if strings.TrimSpace(user) == "" {
			user = "<user>"
		}
		return []string{
			"The background service is per-user, so it needs a user session bus.",
			"  - Do not run this with sudo. Run it as the user that should own the service.",
			"  - On a server that issues no login sessions (sshd with UsePAM no, cron, su)",
			"    give the user a lasting one, then run this again:",
			"        sudo loginctl enable-linger " + user,
		}
	}

	return nil
}
