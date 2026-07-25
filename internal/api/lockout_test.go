package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lockoutBody() string {
	return `{"title":"Your plan has lapsed","detail":"Reactivate your plan to make changes.","status":402,"reason":"subscription_required"}`
}

func serving(t *testing.T, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "token")
}

func TestExtractReason_ProblemDetailsExtension_IsRead(t *testing.T) {
	if got := extractReason([]byte(lockoutBody())); got != ReasonSubscriptionRequired {
		t.Fatalf("expected the reason extension, got %q", got)
	}
}

func TestExtractReason_NoReasonInTheBody_IsEmpty(t *testing.T) {
	if got := extractReason([]byte(`{"title":"Unknown technology","status":400}`)); got != "" {
		t.Fatalf("expected no reason, got %q", got)
	}
}

func TestExtractReason_NonJsonBody_IsEmpty(t *testing.T) {
	if got := extractReason([]byte("<html>502 Bad Gateway</html>")); got != "" {
		t.Fatalf("expected no reason, got %q", got)
	}
}

// A proxy can swallow the body, and a status with no explanation still has to
// say something a person can act on, the same way 401 does.
func TestExtractMessage_PaymentRequiredWithNoBody_ExplainsTheLapsedPlan(t *testing.T) {
	got := extractMessage(nil, http.StatusPaymentRequired)
	if !strings.Contains(got, "lapsed") {
		t.Fatalf("expected a lapsed plan hint, got %q", got)
	}
}

func TestExtractMessage_PaymentRequiredWithABody_PrefersTheServersWording(t *testing.T) {
	got := extractMessage([]byte(lockoutBody()), http.StatusPaymentRequired)
	if got != "Reactivate your plan to make changes." {
		t.Fatalf("expected the detail from the body, got %q", got)
	}
}

func TestDo_ProblemWithAReason_CarriesItOnTheError(t *testing.T) {
	err := serving(t, http.StatusPaymentRequired, lockoutBody()).SetKernel(1, "6.8.0")

	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected an api error, got %T", err)
	}
	if apiErr.Reason != ReasonSubscriptionRequired {
		t.Fatalf("expected the reason kept on the error, got %q", apiErr.Reason)
	}
}

func TestIsSubscriptionLapsed_LockoutRefusal_IsRecognised(t *testing.T) {
	err := serving(t, http.StatusPaymentRequired, lockoutBody()).SetKernel(1, "6.8.0")

	if !IsSubscriptionLapsed(err) {
		t.Fatalf("expected a lapsed plan verdict, got %v", err)
	}
}

// A proxy that drops the body leaves nothing but the status, and a lapsed plan
// is the only thing the CLI's own calls are refused with.
func TestIsSubscriptionLapsed_PaymentRequiredWithNoBody_IsRecognised(t *testing.T) {
	err := serving(t, http.StatusPaymentRequired, "").SetKernel(1, "6.8.0")

	if !IsSubscriptionLapsed(err) {
		t.Fatalf("expected a lapsed plan verdict, got %v", err)
	}
}

// The server answers 402 for more than one billing refusal, so the reason is
// what decides. Reporting an unconfirmed plan change as a lapsed plan would
// send someone to reactivate a plan they already hold.
func TestIsSubscriptionLapsed_DifferentBillingReason_IsNot(t *testing.T) {
	body := `{"title":"Confirm the change","status":402,"reason":"plan_change_not_confirmed"}`
	err := serving(t, http.StatusPaymentRequired, body).SetKernel(1, "6.8.0")

	if IsSubscriptionLapsed(err) {
		t.Fatalf("a different billing reason must not be read as a lapsed plan, got %v", err)
	}
}

func TestIsSubscriptionLapsed_OtherFailures_AreNot(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusUpgradeRequired,
		http.StatusConflict,
		http.StatusNotFound,
		http.StatusInternalServerError,
	} {
		err := serving(t, status, "").SetKernel(1, "6.8.0")

		if IsSubscriptionLapsed(err) {
			t.Fatalf("status %d must not be read as a lapsed plan", status)
		}
	}
}

func TestIsSubscriptionLapsed_NoError_IsFalse(t *testing.T) {
	if IsSubscriptionLapsed(nil) {
		t.Fatal("nil must not be read as a lapsed plan")
	}
}

// Both refusals are final for the call that hit them, but only one of them is
// something the binary can fix by replacing itself.
func TestIsUpgradeRequired_LockoutRefusal_IsNot(t *testing.T) {
	err := serving(t, http.StatusPaymentRequired, lockoutBody()).SetKernel(1, "6.8.0")

	if IsUpgradeRequired(err) {
		t.Fatal("a lapsed plan must not trigger a self upgrade")
	}
}

func TestMe_LockedOutAccount_ReportsTheLockout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":true,"email":"a@b.c","subscriptionLocked":true}`))
	}))
	t.Cleanup(server.Close)

	me, err := New(server.URL, "token").Me()

	if err != nil {
		t.Fatal(err)
	}
	if !me.SubscriptionLocked {
		t.Fatal("expected the lockout flag read from the response")
	}
}

// The flag is absent on an older server, and defaulting to locked would refuse
// every scan against one.
func TestMe_ServerWithoutTheFlag_IsNotLockedOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authenticated":true,"email":"a@b.c"}`))
	}))
	t.Cleanup(server.Close)

	me, err := New(server.URL, "token").Me()

	if err != nil {
		t.Fatal(err)
	}
	if me.SubscriptionLocked {
		t.Fatal("a response with no flag must not be read as locked out")
	}
}
