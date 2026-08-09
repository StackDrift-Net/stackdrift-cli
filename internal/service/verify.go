package service

import "errors"

// Swapped in tests. Install and Status are per-platform, so the only way to
// exercise the verification on every platform is to stand in for them.
var (
	installFunc = Install
	statusFunc  = Status
)

// InstallVerified installs and then reads the scheduler back to confirm it
// actually took the unit.
//
// Install alone is not enough. Every platform here drives its scheduler through
// a command line, and a command that reports success has still only told us it
// ran. On Linux the enable step needs the user bus, and a machine reached over
// ssh or su has no XDG_RUNTIME_DIR, so for months installs wrote their unit
// files, exited zero and left nothing scheduled. The caller recorded that as a
// service the user had agreed to and never asked again, so the machine went
// unwatched with a saved preference saying it was covered.
//
// Anything that returns an error here must leave no preference behind, which is
// why this is what the caller calls rather than Install.
func InstallVerified(plan Plan) error {
	if err := installFunc(plan); err != nil {
		return err
	}

	state, err := statusFunc()
	return verifyInstalled(state, err)
}

// verifyInstalled turns what the scheduler reports into a verdict.
//
// statusErr means the question could not be asked at all. That is deliberately
// NOT a failure: an install is only rejected on a definite answer, because
// treating "unknown" as broken would fail every install on a platform whose
// status probe is unavailable, over evidence nobody has.
func verifyInstalled(state State, statusErr error) error {
	if statusErr != nil {
		return nil
	}

	if !state.Installed {
		return errors.New("the scheduler was not created, so nothing will run")
	}
	if !state.Enabled {
		return errors.New("the scheduler is installed but not enabled, so nothing will run")
	}
	if !state.Running {
		return errors.New("the scheduler is enabled but not running, so nothing will run")
	}
	return nil
}
