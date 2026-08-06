package service

import (
	"errors"
	"os"
	"path/filepath"
)

// Name is shared by every platform so the installed unit, agent or task can be
// found again by a later version of the CLI without guessing.
const Name = "stackdrift-watch"

var ErrUnsupported = errors.New("a background service is not supported on this platform")

// Plan is what to install. Exec is resolved to an absolute path before it is
// written anywhere, because every scheduler here runs without a PATH worth
// relying on and a bare command name would simply fail to start.
//
// AutoUpdate is part of the plan rather than a preference read later, because
// on Linux it changes the unit itself. The sandbox has to be widened by one
// directory for the update to be able to write anything at all.
type Plan struct {
	Interval   string
	Exec       string
	AutoUpdate bool
}

// ScheduledFlag marks the run the scheduler starts, so it can be told apart
// from a person typing "stackdrift watch". Nothing else distinguishes them, and
// only the former has been agreed to replace an executable.
const ScheduledFlag = "--scheduled"

// State is what the platform reports back. Installed says the unit exists;
// Running says the scheduler has it active. They differ while a service is
// installed but stopped, which is exactly the case a status command exists to
// show.
type State struct {
	Installed bool
	Running   bool
	Interval  string
	Detail    string
}

// Executable resolves the running binary, following a symlink so a service
// installed through a shim in ~/.local/bin keeps working after the shim is
// replaced by an update.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
