package commands

import (
	"errors"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

var errSessionExpired = errors.New("your session is no longer valid, run: stackdrift login")

var errSubscriptionLapsed = errors.New("your StackDrift plan has lapsed, so changes are refused")

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

// IsSubscriptionLapsed reports whether a failure is the lapsed-plan refusal,
// whether it was caught by the pre-flight check or by the server turning a
// write away part way through a run.
func IsSubscriptionLapsed(err error) bool {
	return errors.Is(err, errSubscriptionLapsed) || api.IsSubscriptionLapsed(err)
}

// Only ever called once the server has rejected the token, so the stored copy
// is known to be useless. Leaving it would make the next run repeat the same
// failure instead of prompting a login.
func clearRejectedCredential(baseURL string) {
	_ = config.DeleteCredential(baseURL)
}
