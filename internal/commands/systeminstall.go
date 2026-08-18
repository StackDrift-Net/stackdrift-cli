package commands

import (
	"errors"
	"io/fs"
	"strings"
)

// unreplaceableOutcome says what to do about an install directory that refused a
// write, and it is three answers rather than two because Windows added a case
// the other platforms do not have.
type unreplaceableOutcome int

const (
	// A deliberate system install. Nothing is wrong, the update simply has to be
	// applied by re-running the installer with the rights the directory needs.
	updateNeedsInstaller unreplaceableOutcome = iota
	// Broken, and waiting will not fix it. Stop checking and say so.
	updateBlocked
	// Might clear on its own. Try again at the next check.
	updateDeferred
)

// systemInstallDir reports whether the binary lives under a directory only an
// administrator can write to, put there on purpose by the installer.
//
// The roots are passed in rather than read here so the decision stays pure and
// is tested on every platform. Only Windows supplies any; everywhere else this
// is always false, which is correct, because no other platform's installer puts
// the binary somewhere the user running it cannot write.
func systemInstallDir(dir string, roots []string) bool {
	dir = trimSeparators(dir)
	if dir == "" {
		return false
	}

	for _, root := range roots {
		root = trimSeparators(root)
		if root == "" {
			// An unset ProgramFiles would otherwise be an empty prefix, which
			// every path in the world starts with.
			continue
		}
		if strings.EqualFold(dir, root) {
			return true
		}
		// The match has to end on a separator, or "C:\Program Files Extra" reads
		// as living inside "C:\Program Files" and its updates are skipped for
		// ever with a message about an installer that would not help.
		if len(dir) > len(root) && strings.EqualFold(dir[:len(root)], root) && isSeparator(dir[len(root)]) {
			return true
		}
	}
	return false
}

// filepath.Clean is deliberately not used. It is compiled for the platform
// running the code, so on the machine the suite runs on it would leave Windows
// separators alone and the comparison would be testing something other than what
// Windows does.
func trimSeparators(path string) string {
	return strings.TrimRight(strings.TrimSpace(path), `\/`)
}

func isSeparator(c byte) bool { return c == '\\' || c == '/' }

// classifyUnreplaceable decides what a failed write to the install directory
// means.
//
// Only a permission refusal counts as a managed install. A full disk under
// Program Files is still a full disk, and answering it with "run the installer
// as an administrator" would send someone to do something that cannot fix it.
func classifyUnreplaceable(dir string, roots []string, err error) unreplaceableOutcome {
	if errors.Is(err, fs.ErrPermission) && systemInstallDir(dir, roots) {
		return updateNeedsInstaller
	}
	if permanentlyUnwritable(err) {
		return updateBlocked
	}
	return updateDeferred
}
