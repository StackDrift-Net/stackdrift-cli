package service

// RestartsOnExit reports whether the scheduler starts the resident watcher
// again after it exits, which is how a resident process hands over to a binary
// that has just replaced it.
//
// A resident Windows watcher is registered ONLOGON, so nothing restarts it. It
// therefore never replaces its own binary at all, and service status says so
// rather than promising something that cannot happen. The scheduled task is
// unaffected, being short lived like the other platforms.
func RestartsOnExit() bool { return false }
