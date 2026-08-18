//go:build !windows

package commands

// No other platform's installer puts the binary somewhere the account running it
// cannot write, so a refused write there is a fault rather than a layout.
func programRoots() []string { return nil }
