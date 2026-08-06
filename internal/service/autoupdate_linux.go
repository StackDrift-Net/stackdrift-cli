package service

// RestartsOnExit reports whether the scheduler starts the resident watcher
// again after it exits, which is how a resident process hands over to a binary
// that has just replaced it. A process that exits where nothing will start it
// again has simply stopped watching.
//
// The resident unit unitFile writes carries Restart=always with RestartSec=30.
// Verified against real systemd rather than read off the documentation: a
// Type=simple unit with Restart=always is started again after a CLEAN exit 0,
// not only after a failure.
func RestartsOnExit() bool { return true }
