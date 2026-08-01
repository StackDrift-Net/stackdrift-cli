package commands

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// recordingServer signs in against a server that answers /api/auth/me with the
// given body and records every other path it is asked for, which is what shows
// whether a command stopped before it started working.
func recordingServer(t *testing.T, me string, rest http.HandlerFunc) *[]string {
	t.Helper()
	var mu sync.Mutex
	paths := []string{}

	signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/me" {
			_, _ = w.Write([]byte(me))
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if rest != nil {
			rest(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	return &paths
}

func TestWritableClient_LiveSessionOnAPaidPlan_ReturnsAClient(t *testing.T) {
	recordingServer(t, `{"authenticated":true,"email":"a@b.c"}`, nil)

	client, _, err := writableClient()

	if err != nil {
		t.Fatalf("expected a client for a paid plan, got %v", err)
	}
	if client == nil {
		t.Fatal("expected a client back")
	}
}

func TestWritableClient_LapsedAccount_RefusesBeforeReturningAClient(t *testing.T) {
	recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true,"hasEverSubscribed":true}`, nil)

	client, _, err := writableClient()

	if !errors.Is(err, errSubscriptionLapsed) {
		t.Fatalf("expected the lapsed plan refusal, got %v", err)
	}
	if client != nil {
		t.Fatal("expected no client back for a locked out account")
	}
}

// Locked out is the same verdict either way, but somebody on their first scan
// has not let anything lapse. Telling them so was the CLI half of the same bug
// the website had.
func TestWritableClient_AccountThatNeverSubscribed_DoesNotSayAnythingLapsed(t *testing.T) {
	recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true,"hasEverSubscribed":false}`, nil)

	client, _, err := writableClient()

	if !errors.Is(err, errNoPlan) {
		t.Fatalf("expected the no plan refusal, got %v", err)
	}
	if strings.Contains(err.Error(), "lapsed") {
		t.Fatalf("a new account must not be told anything lapsed, got %q", err)
	}
	if client != nil {
		t.Fatal("expected no client back for a locked out account")
	}
}

// Both are refusals for want of a plan, so both must stop the command with the
// billing exit code rather than being read as an ordinary failure.
func TestIsPlanRequired_NoPlanRefusal_IsRecognised(t *testing.T) {
	if !IsPlanRequired(errNoPlan) {
		t.Fatal("expected the no plan refusal recognised")
	}
}

func TestPlanHint_NeverSubscribed_OffersAPlanRatherThanAReactivation(t *testing.T) {
	hint := PlanHint(errNoPlan, "https://stackdrift.net")

	if strings.Contains(hint, "Reactivate") {
		t.Fatalf("a new account has nothing to reactivate, got %q", hint)
	}
	if !strings.Contains(hint, "https://stackdrift.net/billing") {
		t.Fatalf("expected the billing page in the hint, got %q", hint)
	}
}

// The other half: somebody who really did lapse still needs to know their reads
// and removes are open, which is how they get back under a smaller plan.
func TestPlanHint_Lapsed_StillSaysReadsAndRemovesWork(t *testing.T) {
	hint := PlanHint(errSubscriptionLapsed, "https://stackdrift.net")

	if !strings.Contains(hint, "Reading and removing still work") {
		t.Fatalf("expected the read and remove reassurance, got %q", hint)
	}
}

// A lapsed plan says nothing about the token, so signing the account out of the
// CLI would make paying up harder than it needs to be.
func TestWritableClient_LockedOutAccount_KeepsTheCredential(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`))
	})

	if _, _, err := writableClient(); err == nil {
		t.Fatal("expected the lapsed plan refusal")
	}

	if token := storedToken(t, baseURL); token != "stored-token" {
		t.Fatalf("expected the credential kept, got %q", token)
	}
}

func TestWritableClient_DeadSession_ReportsTheSessionNotThePlan(t *testing.T) {
	recordingServer(t, `{"authenticated":false,"subscriptionLocked":true}`, nil)

	_, _, err := writableClient()

	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("expected the expired session error, got %v", err)
	}
}

// The point of the pre-flight: scan walks the whole filesystem and asks two
// questions before it writes anything, and applying can fail part way through.
func TestScan_LockedOutAccount_StopsBeforeTouchingAnything(t *testing.T) {
	paths := recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`, nil)

	err := Scan([]string{"--yes"})

	if !IsPlanRequired(err) {
		t.Fatalf("expected the lapsed plan refusal, got %v", err)
	}
	if len(*paths) != 0 {
		t.Fatalf("expected scan to stop before calling anything else, it called %v", *paths)
	}
}

// Reading is deliberately left open, because seeing what you have is how you
// work out what to remove so a smaller plan will take you.
func TestCheck_LockedOutAccount_StillReportsCveStatus(t *testing.T) {
	dir := t.TempDir()
	stats := `{"technologyCount":1,"endOfLifeCount":0,"technologyCveCount":0,"dependencyCount":0,"vulnerableDependencyCount":0,"dependencyCveCount":0}`
	recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(stats))
	})
	if err := config.SaveProject(dir, &config.ProjectConfig{ProjectID: 7, ProjectName: "Locked"}); err != nil {
		t.Fatal(err)
	}

	client, _, err := authenticatedClient()
	if err != nil {
		t.Fatalf("a locked out account must still get a client for reading, got %v", err)
	}

	if err := check(client, dir); err != nil {
		t.Fatalf("expected check to run for a locked out account, got %v", err)
	}
}

func TestCheck_LockedOutAccountWithACve_StillFailsForTheCve(t *testing.T) {
	dir := t.TempDir()
	stats := `{"technologyCount":1,"endOfLifeCount":0,"technologyCveCount":2,"dependencyCount":0,"vulnerableDependencyCount":0,"dependencyCveCount":0}`
	recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(stats))
	})
	if err := config.SaveProject(dir, &config.ProjectConfig{ProjectID: 7, ProjectName: "Locked"}); err != nil {
		t.Fatal(err)
	}

	client, _, err := authenticatedClient()
	if err != nil {
		t.Fatal(err)
	}

	err = check(client, dir)

	var cveErr *CveFoundError
	if !errors.As(err, &cveErr) {
		t.Fatalf("expected the CVE result, got %v", err)
	}
	if IsPlanRequired(err) {
		t.Fatal("a CVE result must never be reported as a lapsed plan")
	}
}

func TestIsPlanRequired_PreflightRefusal_IsRecognised(t *testing.T) {
	if !IsPlanRequired(errSubscriptionLapsed) {
		t.Fatal("expected the pre-flight refusal recognised")
	}
}

// A token revoked mid-run and a write refused mid-run are different failures
// with different answers, so neither may be read as the other.
func TestIsPlanRequired_ExpiredSession_IsNot(t *testing.T) {
	if IsPlanRequired(errSessionExpired) {
		t.Fatal("an expired session must not be read as a lapsed plan")
	}
}

func TestIsPlanRequired_ServerRefusedAWriteMidRun_IsRecognised(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"status":402,"reason":"subscription_required"}`))
	})

	err := api.New(baseURL, "stored-token").SetKernel(3, "6.8.0")

	if !IsPlanRequired(err) {
		t.Fatalf("expected the server refusal recognised, got %v", err)
	}
}

func TestIsPlanRequired_NoError_IsFalse(t *testing.T) {
	if IsPlanRequired(nil) {
		t.Fatal("nil must not be read as a lapsed plan")
	}
}

// ExpireSession runs on every failure, and a lapsed plan is not a dead session.
// Clearing the token here would sign someone out for owing money.
func TestExpireSession_LapsedPlan_IsLeftAloneAndKeepsTheCredential(t *testing.T) {
	baseURL := signedIn(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"status":402,"reason":"subscription_required"}`))
	})

	callErr := api.New(baseURL, "stored-token").SetKernel(3, "6.8.0")

	err := ExpireSession(callErr)

	if !IsPlanRequired(err) {
		t.Fatalf("expected the refusal to pass through unchanged, got %v", err)
	}
	if token := storedToken(t, baseURL); token != "stored-token" {
		t.Fatalf("expected the credential kept, got %q", token)
	}
}

func TestScan_LockedOutAccount_DoesNotWriteAProjectLink(t *testing.T) {
	recordingServer(t, `{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`, nil)
	home, err := config.StoreDir()
	if err != nil {
		t.Fatal(err)
	}

	_ = Scan([]string{"--yes"})

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("expected no project link written, found %q", entry.Name())
		}
	}
}
