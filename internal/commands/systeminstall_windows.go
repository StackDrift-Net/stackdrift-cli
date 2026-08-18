package commands

import "os"

// programRoots is where the Windows installer is allowed to put the binary and
// have a refused write mean "managed" rather than "broken".
//
// All three variables are read because which one holds the real 64-bit directory
// depends on the bitness of the process asking. A 32-bit process sees
// ProgramFiles as the (x86) directory and ProgramW6432 as the other one.
func programRoots() []string {
	var roots []string
	for _, key := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if value := os.Getenv(key); value != "" {
			roots = append(roots, value)
		}
	}
	if len(roots) == 0 {
		// A service started with a scrubbed environment still has to reach the
		// right answer, and this path has not moved in thirty years.
		roots = append(roots, `C:\Program Files`)
	}
	return roots
}
