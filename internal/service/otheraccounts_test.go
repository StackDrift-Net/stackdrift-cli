package service

import (
	"strings"
	"testing"
)

const passwd = `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
sync:x:4:65534:sync:/bin:/bin/sync
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
ubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash
deploy:x:1001:1001::/home/deploy:/bin/sh
`

func homeFor(t *testing.T, accounts []accountHome, name string) string {
	t.Helper()
	for _, a := range accounts {
		if a.Account == name {
			return a.Home
		}
	}
	return ""
}

func TestHomesFromPasswd_ReadsEveryRealAccount(t *testing.T) {
	accounts := homesFromPasswd(passwd, "/root")

	if got := homeFor(t, accounts, "ubuntu"); got != "/home/ubuntu" {
		t.Fatalf("got %q", got)
	}
	if got := homeFor(t, accounts, "deploy"); got != "/home/deploy" {
		t.Fatalf("got %q", got)
	}
}

// The account running the command has already been asked about its own
// service, so naming it back would read as a second install that is not there.
func TestHomesFromPasswd_TheAccountAsking_IsLeftOut(t *testing.T) {
	if got := homeFor(t, homesFromPasswd(passwd, "/root"), "root"); got != "" {
		t.Fatal("the current account must not be reported to itself")
	}
}

// A home nothing can be installed under is noise, and /usr/sbin or /bin would
// otherwise be checked once per system account on every install.
func TestHomesFromPasswd_AccountsWithNoRealHome_AreSkipped(t *testing.T) {
	accounts := homesFromPasswd(passwd, "/root")

	for _, unwanted := range []string{"daemon", "sync", "nobody"} {
		if got := homeFor(t, accounts, unwanted); got != "" {
			t.Fatalf("%s has no home worth checking, got %q", unwanted, got)
		}
	}
}

func TestHomesFromPasswd_MalformedLines_AreIgnored(t *testing.T) {
	accounts := homesFromPasswd("nonsense\n\n:::\nubuntu:x:1000:1000:U:/home/ubuntu:/bin/sh\n", "/root")

	if len(accounts) != 1 || accounts[0].Account != "ubuntu" {
		t.Fatalf("got %+v", accounts)
	}
}

// The same home listed twice, which happens with an alias account, must not
// produce the same warning twice.
func TestHomesFromPasswd_TheSameHomeTwice_IsReportedOnce(t *testing.T) {
	both := "a:x:1000:1000::/home/shared:/bin/sh\nb:x:1001:1001::/home/shared:/bin/sh\n"

	if accounts := homesFromPasswd(both, "/root"); len(accounts) != 1 {
		t.Fatalf("got %+v", accounts)
	}
}

// Windows registers one task for the whole machine, so a second account does
// not add a watcher, it takes the existing one over. That is worth saying, and
// it needs the owner read out of the verbose query.
func TestRunAsUserFromTask_ReadsTheOwner(t *testing.T) {
	output := "TaskName:      \\StackDrift Watch\r\nRun As User:   UBUNTU\r\nStatus:        Ready\r\n"

	if got := runAsUserFromTask(output); got != "UBUNTU" {
		t.Fatalf("got %q", got)
	}
}

func TestRunAsUserFromTask_FieldAbsent_IsEmpty(t *testing.T) {
	if got := runAsUserFromTask("TaskName: \\StackDrift Watch\r\nStatus: Ready\r\n"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestOtherAccountLines_NothingFound_SaysNothing(t *testing.T) {
	if lines := OtherAccountLines(nil, "root"); len(lines) != 0 {
		t.Fatalf("a clean machine must print nothing, got %v", lines)
	}
}

func TestOtherAccountLines_OneFound_NamesTheAccountAndWhere(t *testing.T) {
	found := []Installation{{Account: "ubuntu", Detail: "/home/ubuntu/.config/systemd/user/stackdrift-watch.service"}}

	joined := strings.Join(OtherAccountLines(found, "root"), "\n")
	if !strings.Contains(joined, "ubuntu") {
		t.Fatalf("expected the account named:\n%s", joined)
	}
	if !strings.Contains(joined, "/home/ubuntu/.config/systemd/user/stackdrift-watch.service") {
		t.Fatalf("expected the location shown:\n%s", joined)
	}
}

func TestOtherAccountLines_SeveralFound_NamesThemAll(t *testing.T) {
	found := []Installation{
		{Account: "ubuntu", Detail: "/home/ubuntu/x"},
		{Account: "deploy", Detail: "/home/deploy/x"},
	}

	joined := strings.Join(OtherAccountLines(found, "root"), "\n")
	if !strings.Contains(joined, "ubuntu") || !strings.Contains(joined, "deploy") {
		t.Fatalf("expected both named:\n%s", joined)
	}
}

// A watcher only sweeps what its own account scanned, so the warning has to say
// that rather than reading as "this will not work".
func TestOtherAccountLines_Always_ExplainsWhenItMatters(t *testing.T) {
	found := []Installation{{Account: "ubuntu", Detail: "/home/ubuntu/x"}}

	joined := strings.Join(OtherAccountLines(found, "root"), "\n")
	if !strings.Contains(joined, "own account") {
		t.Fatalf("expected the scope explained:\n%s", joined)
	}
}

func TestOtherAccountLines_OneFound_ReadsAsSingular(t *testing.T) {
	lines := OtherAccountLines([]Installation{{Account: "ubuntu", Detail: "/x"}}, "root")

	if lines[0] != "Another account on this machine already has a StackDrift watcher." {
		t.Fatalf("got %q", lines[0])
	}
}

func TestOtherAccountLines_SeveralFound_ReadsAsPlural(t *testing.T) {
	lines := OtherAccountLines([]Installation{
		{Account: "ubuntu", Detail: "/x"},
		{Account: "deploy", Detail: "/y"},
	}, "root")

	if lines[0] != "Other accounts on this machine already have StackDrift watchers." {
		t.Fatalf("got %q", lines[0])
	}
}
