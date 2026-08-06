package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssetName_PerPlatform(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "amd64"}:   "stackdrift-linux-amd64",
		{"linux", "arm64"}:   "stackdrift-linux-arm64",
		{"darwin", "arm64"}:  "stackdrift-darwin-arm64",
		{"windows", "amd64"}: "stackdrift-windows-amd64.exe",
		{"windows", "arm64"}: "stackdrift-windows-arm64.exe",
	}
	for in, want := range cases {
		if got := assetName(in[0], in[1]); got != want {
			t.Errorf("assetName(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestNeedsUpdate_Cases(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"dev", "v0.1.0", true},
		{"", "v0.1.0", true},
		{"0.1.0", "v0.1.0", false},
		{"v0.1.0", "0.1.0", false},
		{"0.1.0", "v0.2.0", true},
		{" 0.1.0 ", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := needsUpdate(c.current, c.latest); got != c.want {
			t.Errorf("needsUpdate(%q,%q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestParseLatestTag(t *testing.T) {
	tag, err := parseLatestTag([]byte(`{"tag_name":"v0.3.0","name":"0.3.0"}`))
	if err != nil || tag != "v0.3.0" {
		t.Fatalf("expected v0.3.0, got %q (err %v)", tag, err)
	}

	if _, err := parseLatestTag([]byte(`{"tag_name":""}`)); err == nil {
		t.Fatal("expected error for empty tag")
	}
	if _, err := parseLatestTag([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestFetchLatestTag_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	tag, err := fetchLatestTag(srv.URL, "owner/repo")
	if err != nil || tag != "v1.2.3" {
		t.Fatalf("expected v1.2.3, got %q (err %v)", tag, err)
	}
}

func TestFetchLatestTag_NotFound_ExplainsPrivateOrNoRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchLatestTag(srv.URL, "owner/repo")
	if err == nil {
		t.Fatal("expected an error for 404")
	}
}
