package commands

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// signedIn points the CLI at a throwaway server and stores a credential for it,
// with both the home directory and the server isolated to this test.
func signedIn(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	// Credentials live in the user config directory rather than the store, so
	// that is isolated too. Without it a test run appends throwaway tokens to
	// the credentials file of whoever is running it.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("STACKDRIFT_URL", server.URL)

	if err := config.SaveCredential(config.Credential{
		BaseURL: server.URL,
		Token:   "stored-token",
		Email:   "someone@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	return server.URL
}

func storedToken(t *testing.T, baseURL string) string {
	t.Helper()
	cred, err := config.LoadCredential(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if cred == nil {
		return ""
	}
	return cred.Token
}

func TestValidateSession_LiveSession_ReturnsTheAccount(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":true,"email":"someone@example.com"}`))
	})

	me, err := validateSession(api.New(baseURL, "stored-token"), baseURL)

	if err != nil {
		t.Fatalf("expected a live session, got %v", err)
	}
	if me.Email != "someone@example.com" {
		t.Fatalf("expected the account back, got %q", me.Email)
	}
}

// The endpoint is anonymous and answers 200 for a token the server will not
// accept, so the flag is the verdict rather than the status code.
func TestValidateSession_RejectedToken_ReportsExpiredAndClearsIt(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":false}`))
	})

	_, err := validateSession(api.New(baseURL, "stored-token"), baseURL)

	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("expected the expired session error, got %v", err)
	}
	if token := storedToken(t, baseURL); token != "" {
		t.Fatalf("expected the rejected credential to be cleared, still holding %q", token)
	}
}

// Being unable to reach the server says nothing about the token. Clearing it
// here would sign someone out for working offline.
func TestValidateSession_ServerError_KeepsTheCredential(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := validateSession(api.New(baseURL, "stored-token"), baseURL)

	if err == nil {
		t.Fatal("expected the server error to surface")
	}
	if errors.Is(err, errSessionExpired) {
		t.Fatal("a server error must not be read as an expired session")
	}
	if token := storedToken(t, baseURL); token != "stored-token" {
		t.Fatalf("expected the credential kept, got %q", token)
	}
}

// Every endpoint other than me is authorized, so a token revoked mid-run is
// rejected with 401 rather than the anonymous flag.
func TestExpireSession_Unauthorized_ReportsExpiredAndClearsIt(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	client := api.New(baseURL, "stored-token")
	_, callErr := client.ListProjects()

	err := ExpireSession(callErr)

	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("expected the expired session error, got %v", err)
	}
	if token := storedToken(t, baseURL); token != "" {
		t.Fatalf("expected the rejected credential to be cleared, still holding %q", token)
	}
}

func TestExpireSession_OtherFailure_IsLeftAlone(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	client := api.New(baseURL, "stored-token")
	_, callErr := client.ListProjects()

	err := ExpireSession(callErr)

	if errors.Is(err, errSessionExpired) {
		t.Fatal("a 404 must not be read as an expired session")
	}
	if token := storedToken(t, baseURL); token != "stored-token" {
		t.Fatalf("expected the credential kept, got %q", token)
	}
}

func TestExpireSession_NoError_StaysNil(t *testing.T) {
	if err := ExpireSession(nil); err != nil {
		t.Fatalf("expected nil to pass through, got %v", err)
	}
}

func TestAuthenticatedSession_NoCredential_AsksForLogin(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)
	t.Setenv("STACKDRIFT_URL", server.URL)

	_, _, _, err := authenticatedSession()

	if !errors.Is(err, errNotSignedIn) {
		t.Fatalf("expected the not-signed-in guard, got %v", err)
	}
	if called {
		t.Fatal("expected no request when there is no credential to check")
	}
}

// The check has to happen before a command starts, because the expensive part
// of a run is local: scan walks the filesystem before it calls the API.
func TestAuthenticatedSession_RejectedToken_FailsBeforeReturningAClient(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":false}`))
	})

	client, _, _, err := authenticatedSession()

	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("expected the expired session error, got %v", err)
	}
	if client != nil {
		t.Fatal("expected no client back for a dead session")
	}
	if token := storedToken(t, baseURL); token != "" {
		t.Fatalf("expected the rejected credential to be cleared, still holding %q", token)
	}
}
