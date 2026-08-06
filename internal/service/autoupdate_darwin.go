package service

// RestartsOnExit reports whether the scheduler starts the resident watcher
// again after it exits, which is how a resident process hands over to a binary
// that has just replaced it.
//
// The resident agent plistBody writes carries KeepAlive, which is launchd's
// instruction to start the job again whenever it stops for any reason.
func RestartsOnExit() bool { return true }
