package commands

import (
	"errors"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

var errSessionExpired = errors.New("your session is no longer valid, run: stackdrift login")

var errSubscriptionLapsed = errors.New("your StackDrift plan has lapsed, so changes are refused")

// The same refusal, for an account that has never held a plan at all. There is
// nothing lapsed about it, and saying so to somebody on their first scan is
// simply wrong.
var errNoPlan = errors.New("your StackDrift account has no active plan, so changes are refused")

// A token lasts 90 days and can be revoked from the website at any time, so
// holding one on disk is not proof of a session. Commands are checked before
// they start because the expensive part of a run is local: scan walks the whole
// filesystem before it ever calls the API, and failing at the end of that is a
// worse answer than failing immediately.
func validateSession(client *api.Client, baseURL string) (*api.Me, error) {
	me, err := client.Me()
	if err != nil {
		// A refused connection or a 500 says nothing about the token. Treating
		// it as a dead session would sign people out for being offline.
		return nil, err
	}

	// This endpoint is anonymous and answers 200 with Authenticated false for a
	// token the server will not accept, so the flag is the verdict, not the
	// status code.
	if !me.Authenticated {
		clearRejectedCredential(baseURL)
		return nil, errSessionExpired
	}

	return me, nil
}

// ExpireSession converts a rejection from any later call into the same answer
// the startup check gives. Every other endpoint is authorized, so those reject
// with 401 rather than the anonymous flag, and a token can be revoked between
// the check and the call that uses it.
func ExpireSession(err error) error {
	if err == nil || !api.IsUnauthorized(err) {
		return err
	}

	clearRejectedCredential(config.BaseURL())
	return errSessionExpired
}

// IsPlanRequired reports whether a failure is the no-plan refusal, whether it
// was caught by the pre-flight check or by the server turning a write away part
// way through a run. Both wordings answer yes, because the exit code is about
// what stopped the command, not about who the account is.
func IsPlanRequired(err error) bool {
	return errors.Is(err, errSubscriptionLapsed) || errors.Is(err, errNoPlan) || api.IsSubscriptionLapsed(err)
}

// What to say after the refusal. Keeping the reads and removes open is worth
// telling somebody who has projects to read, and an account that never
// subscribed has none, so all it needs is where to go.
func PlanHint(err error, baseURL string) string {
	if errors.Is(err, errNoPlan) {
		return "Pick a plan at " + baseURL + "/billing"
	}
	return "Reading and removing still work. Reactivate your plan at " + baseURL + "/billing"
}

// Only ever called once the server has rejected the token, so the stored copy
// is known to be useless. Leaving it would make the next run repeat the same
// failure instead of prompting a login.
func clearRejectedCredential(baseURL string) {
	_ = config.DeleteCredential(baseURL)
}
