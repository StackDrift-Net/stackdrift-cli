package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestFinishLogin_StoresTheTokenAgainstTheServerAndAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"authenticated":true,"email":"vince@example.com","userId":"u1"}`))
	}))
	defer server.Close()

	if err := finishLogin(server.URL, "sdp_new"); err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer sdp_new" {
		t.Fatalf("expected the fresh token used to read the account, got %q", gotAuth)
	}

	saved, err := config.LoadCredential(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil {
		t.Fatal("expected the credential saved")
	}
	if saved.Token != "sdp_new" || saved.Email != "vince@example.com" {
		t.Fatalf("expected the token and account recorded, got %+v", saved)
	}
}

func TestFinishLogin_AccountUnreadable_ExplainsAndSavesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	err := finishLogin(server.URL, "sdp_rejected")

	if err == nil || !strings.Contains(err.Error(), "could not read account") {
		t.Fatalf("expected a clear failure, got %v", err)
	}

	// A token the server will not accept must not be left behind, or every
	// later command fails with a confusing 401 instead of asking for a login.
	saved, loadErr := config.LoadCredential(server.URL)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if saved != nil {
		t.Fatalf("expected no credential stored after a failed login, got %+v", saved)
	}
}

func TestFinishLogin_SecondServer_DoesNotDisturbTheFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	account := func(email string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"authenticated":true,"email":"` + email + `"}`))
		}))
	}

	first := account("a@example.com")
	defer first.Close()
	second := account("b@example.com")
	defer second.Close()

	if err := finishLogin(first.URL, "tok-first"); err != nil {
		t.Fatal(err)
	}
	if err := finishLogin(second.URL, "tok-second"); err != nil {
		t.Fatal(err)
	}

	kept, err := config.LoadCredential(first.URL)
	if err != nil {
		t.Fatal(err)
	}
	if kept == nil || kept.Token != "tok-first" {
		t.Fatalf("expected the first server's credential intact, got %+v", kept)
	}
}
