package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
)

// captureSleeps replaces the polling wait so tests run instantly, and records
// what the loop would have waited for.
func captureSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	original := sleep
	sleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { sleep = original })
	return &slept
}

func pollAgainst(t *testing.T, handler http.HandlerFunc, auth *api.DeviceAuthorization) (string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return poll(api.New(server.URL, ""), auth)
}

func deviceAuth() *api.DeviceAuthorization {
	return &api.DeviceAuthorization{DeviceCode: "dc", IntervalSeconds: 1, ExpiresInSeconds: 600}
}

func TestPoll_Approved_ReturnsAccessToken(t *testing.T) {
	captureSleeps(t)

	token, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"sdp_live","tokenType":"Bearer"}`))
	}, deviceAuth())

	if err != nil {
		t.Fatal(err)
	}
	if token != "sdp_live" {
		t.Fatalf("expected the access token, got %q", token)
	}
}

func TestPoll_PendingThenApproved_KeepsPolling(t *testing.T) {
	captureSleeps(t)
	calls := 0

	token, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"sdp_after_wait"}`))
	}, deviceAuth())

	if err != nil {
		t.Fatal(err)
	}
	if token != "sdp_after_wait" {
		t.Fatalf("expected the token after pending responses, got %q", token)
	}
	if calls != 3 {
		t.Fatalf("expected to poll until approved, got %d calls", calls)
	}
}

func TestPoll_Denied_ReportsDenial(t *testing.T) {
	captureSleeps(t)

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}, deviceAuth())

	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected a denial error, got %v", err)
	}
}

func TestPoll_Gone_ReportsExpiry(t *testing.T) {
	captureSleeps(t)

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}, deviceAuth())

	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected an expiry error, got %v", err)
	}
}

func TestPoll_UnexpectedStatus_StopsRatherThanLooping(t *testing.T) {
	captureSleeps(t)
	calls := 0

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}, deviceAuth())

	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("expected an unexpected-status error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected to give up after one bad status, got %d calls", calls)
	}
}

func TestPoll_TransportError_RetriesInsteadOfFailing(t *testing.T) {
	captureSleeps(t)
	calls := 0

	token, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Drop the connection so the client sees a transport error, which
			// is a blip rather than a decision from the server.
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("expected a hijackable response writer")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"sdp_recovered"}`))
	}, deviceAuth())

	if err != nil {
		t.Fatal(err)
	}
	if token != "sdp_recovered" {
		t.Fatalf("expected to recover after a dropped connection, got %q", token)
	}
}

func TestPoll_AlreadyExpired_TimesOutWithoutCallingTheServer(t *testing.T) {
	captureSleeps(t)
	calls := 0

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
	}, &api.DeviceAuthorization{DeviceCode: "dc", IntervalSeconds: 1, ExpiresInSeconds: 0})

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no polling past the deadline, got %d calls", calls)
	}
}

func TestPoll_UsesServerInterval(t *testing.T) {
	slept := captureSleeps(t)

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"t"}`))
	}, &api.DeviceAuthorization{DeviceCode: "dc", IntervalSeconds: 7, ExpiresInSeconds: 600})

	if err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 7*time.Second {
		t.Fatalf("expected to honour the server interval, got %v", *slept)
	}
}

func TestPoll_MissingInterval_FallsBackToFiveSeconds(t *testing.T) {
	slept := captureSleeps(t)

	_, err := pollAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"t"}`))
	}, &api.DeviceAuthorization{DeviceCode: "dc", IntervalSeconds: 0, ExpiresInSeconds: 600})

	if err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 5*time.Second {
		t.Fatalf("expected the five second fallback, got %v", *slept)
	}
}
