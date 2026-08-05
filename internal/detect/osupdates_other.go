//go:build !windows

package detect

// Only Windows is asked. Linux has a package manager per distribution with no
// answer in common, and apt on its own would be a feature rather than a line,
// so everywhere else is told nobody could ask rather than nothing is waiting.
func pendingUpdates(func()) (UpdateStatus, bool) { return UpdateStatus{}, false }
