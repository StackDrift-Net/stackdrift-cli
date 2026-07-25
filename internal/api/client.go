package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VersionHeader tells the server which build is calling. The server turns away
// anything behind the current release, so a build that does not send this is
// read as predating the check and refused with it.
const VersionHeader = "X-StackDrift-CLI-Version"

// Set from main at startup, where the release version is stamped in.
var Version = "dev"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// ReasonSubscriptionRequired is the "reason" the server puts on the body when a
// write is refused because the plan has lapsed. The same status carries other
// billing refusals, so the reason is what tells them apart.
const ReasonSubscriptionRequired = "subscription_required"

type Error struct {
	Status  int
	Message string
	Reason  string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("request failed with status %d", e.Status)
}

// IsUpgradeRequired reports whether the server refused the build itself rather
// than anything about the request, which no retry of the same binary can fix.
func IsUpgradeRequired(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUpgradeRequired
}

// IsUnauthorized reports whether the server rejected the token outright. Note
// that /api/auth/me is anonymous and answers 200 with Authenticated false
// instead, so a session check has to read that flag rather than call this.
func IsUnauthorized(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

// IsSubscriptionLapsed reports whether the server refused a write because the
// account's plan has fully lapsed, which no retry can fix and which reads and
// deletes are deliberately not subject to.
func IsSubscriptionLapsed(err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusPaymentRequired {
		return false
	}
	// A body naming a different billing reason is a different refusal, such as
	// an unconfirmed plan change. A 402 carrying no reason came from something
	// that dropped the extension, and a lapsed plan is the only thing the CLI's
	// own calls can be refused with.
	return apiErr.Reason == "" || apiErr.Reason == ReasonSubscriptionRequired
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(VersionHeader, Version)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{
			Status:  resp.StatusCode,
			Message: extractMessage(data, resp.StatusCode),
			Reason:  extractReason(data),
		}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) raw(method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

func marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func unmarshal(data []byte, out any) error {
	return json.Unmarshal(data, out)
}

func extractMessage(data []byte, status int) string {
	var problem struct {
		Message string   `json:"message"`
		Detail  string   `json:"detail"`
		Title   string   `json:"title"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &problem); err == nil {
		switch {
		case problem.Message != "":
			return problem.Message
		case problem.Detail != "":
			return problem.Detail
		case problem.Title != "":
			return problem.Title
		case len(problem.Errors) > 0:
			return strings.Join(problem.Errors, " ")
		}
	}
	if status == http.StatusUnauthorized {
		return "not signed in, run: stackdrift login"
	}
	if status == http.StatusPaymentRequired {
		return "your StackDrift plan has lapsed, so changes are refused"
	}
	return fmt.Sprintf("request failed with status %d", status)
}

// The reason is an RFC 9457 extension rather than a member, so it is read on
// its own instead of widening what picks the message to show.
func extractReason(data []byte) string {
	var problem struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(data, &problem); err != nil {
		return ""
	}
	return problem.Reason
}
