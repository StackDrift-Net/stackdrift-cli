package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// How long the candidate gets to print its own version. It is a local process
// doing nothing but formatting a string, so anything approaching this means it
// is not the program we asked for.
const trialRunBudget = 20 * time.Second

// Executable file headers, one per platform we publish for. A release asset
// that is not one of these is not a binary, whatever the server said the status
// was: a proxy, a captive portal or a moved release all answer 200 with HTML.
var executableMagic = map[string][][]byte{
	"linux": {
		{0x7f, 'E', 'L', 'F'},
	},
	"windows": {
		{'M', 'Z'},
	},
	"darwin": {
		{0xcf, 0xfa, 0xed, 0xfe}, // 64 bit, little endian
		{0xfe, 0xed, 0xfa, 0xcf}, // 64 bit, big endian
		{0xce, 0xfa, 0xed, 0xfe}, // 32 bit, little endian
		{0xfe, 0xed, 0xfa, 0xce}, // 32 bit, big endian
		{0xca, 0xfe, 0xba, 0xbe}, // universal
		{0xbe, 0xba, 0xfe, 0xca}, // universal, byte swapped
	},
}

func hasExecutableMagic(path, goos string) error {
	wanted, known := executableMagic[goos]
	if !known {
		return fmt.Errorf("no way to recognise an executable for %s", goos)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 4)
	read, err := file.Read(header)
	if err != nil && read == 0 {
		return errors.New("the downloaded file is empty")
	}
	header = header[:read]

	for _, magic := range wanted {
		if len(header) >= len(magic) && bytes.Equal(header[:len(magic)], magic) {
			return nil
		}
	}
	return fmt.Errorf("the downloaded file is not a %s executable", goos)
}

// trialRun makes the candidate prove it is the release it claims to be, by
// running it before it is allowed to replace anything.
//
// This is what stops an unattended machine bricking itself. A binary that will
// not start is unrecoverable in a background service: the program that would
// fetch a working one is the program that was overwritten.
func trialRun(path, tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), trialRunBudget)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "version")
	// Killing the candidate is not enough on its own. A descendant holding the
	// inherited pipe keeps CombinedOutput waiting, which would hang the whole
	// sweep, and the oneshot unit has no start timeout to catch it.
	cmd.WaitDelay = 2 * time.Second

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("the downloaded binary would not run: %w", err)
	}

	want := normalizeVersion(tag)
	if want == "" || !strings.Contains(string(output), want) {
		return fmt.Errorf("the downloaded binary reports %q rather than %s",
			strings.TrimSpace(string(output)), tag)
	}
	return nil
}

// previousPath is where the binary being replaced is kept, on the same
// filesystem so it can be moved back with a rename.
//
// Named so no shell will ever pick it up for "stackdrift", and fixed rather
// than timestamped so a machine updating for years accumulates one file and not
// one per release.
func previousPath(exe string) string {
	return exe + ".prev"
}
