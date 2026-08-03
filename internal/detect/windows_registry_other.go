//go:build !windows

package detect

// Nothing to read off Windows. scanHost never asks, this only keeps the
// package building everywhere
func readWindowsInfo() windowsInfo { return windowsInfo{} }
