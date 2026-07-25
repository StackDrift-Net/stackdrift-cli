package commands

import (
	"errors"
	"net/http"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

var errNotSignedIn = errors.New("not signed in, run: stackdrift login")

var errNoProjectLink = errors.New("this directory is not linked to a StackDrift project, run: stackdrift scan")

func isNotFound(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func authenticatedClient() (*api.Client, string, error) {
	client, baseURL, _, err := authenticatedSession()
	return client, baseURL, err
}

// writableClient is for the commands that change the project. A lapsed plan is
// read from the session check that already runs, for the same reason the token
// is: scan walks the whole filesystem and asks two questions before it writes
// anything, and applying can fail part way and leave the project half updated.
// Reads and deletes stay open when locked out, so they must not use this.
func writableClient() (*api.Client, string, error) {
	client, baseURL, me, err := authenticatedSession()
	if err != nil {
		return nil, baseURL, err
	}
	if me.SubscriptionLocked {
		return nil, baseURL, errSubscriptionLapsed
	}
	return client, baseURL, nil
}

// Returns the account alongside the client so a command that needs it does not
// pay for a second round trip to ask again.
func authenticatedSession() (*api.Client, string, *api.Me, error) {
	baseURL := config.BaseURL()
	cred, err := config.LoadCredential(baseURL)
	if err != nil {
		return nil, baseURL, nil, err
	}
	if cred == nil || cred.Token == "" {
		return nil, baseURL, nil, errNotSignedIn
	}

	client := api.New(baseURL, cred.Token)
	me, err := validateSession(client, baseURL)
	if err != nil {
		return nil, baseURL, nil, err
	}

	return client, baseURL, me, nil
}
