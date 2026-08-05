package detect

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode/utf16"
)

// UpdateStatus is what a machine's own updater had queued up for it. Security
// is a subset of Pending, not a number beside it.
type UpdateStatus struct {
	Pending  int
	Security int
}

// PendingUpdates asks the machine what its own updater is holding. The second
// return is false when there was nobody to ask, and the caller has to keep that
// apart from an answer of zero: a machine nobody could ask is not a machine
// that is up to date.
//
// onAsk is called only once there is something to ask and the wait is about to
// start, so a machine with nothing to ask stays silent instead of announcing
// work it is not going to do. Same idea as ScanWithProgress: a pause with no
// explanation reads as a hang.
//
// Deliberately not part of Scan. This starts a process and talks to a service
// that can be wedged, while scanHost promises to be instant, so it is called by
// the code that reports a finished check rather than by the code that runs one.
func PendingUpdates(onAsk func()) (UpdateStatus, bool) {
	return pendingUpdates(onAsk)
}

// parseUpdateCounts reads the key=value lines the probe prints. Kept apart from
// the probe itself, with no build constraint, so the rules below are tested on
// every platform.
func parseUpdateCounts(output string) (UpdateStatus, bool) {
	fields := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if key, value, found := strings.Cut(strings.TrimSpace(line), "="); found {
			fields[key] = strings.TrimSpace(value)
		}
	}

	pending, err := strconv.Atoi(fields["pending"])
	if err != nil || pending < 0 {
		return UpdateStatus{}, false
	}

	// Windows answers out of its own cache, so a machine whose update search has
	// never come back reports nothing waiting when the truth is that nobody has
	// looked. A count above zero is trustworthy either way, because it could not
	// have been named otherwise; a zero is only trustworthy once a search has
	// succeeded.
	if pending == 0 && fields["searched"] == "" {
		return UpdateStatus{}, false
	}

	// The total is the number worth having, so a security count that will not
	// read falls back to none rather than discarding the answer.
	security, err := strconv.Atoi(fields["security"])
	if err != nil || security < 0 {
		security = 0
	}
	if security > pending {
		security = pending
	}

	return UpdateStatus{Pending: pending, Security: security}, true
}

// encodeCommand renders a script the way PowerShell's -EncodedCommand wants it,
// base64 over UTF-16 with the low byte first. Untagged so it is tested off
// Windows, in the same spirit as the schtasks string work.
func encodeCommand(script string) string {
	units := utf16.Encode([]rune(script))
	bytes := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		bytes = append(bytes, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
