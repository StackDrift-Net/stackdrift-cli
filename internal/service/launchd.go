package service

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// The parts of the macOS installer that are pure string work live here rather
// than in service_darwin.go, with no build constraint, so their tests run on
// every platform. Only the code that shells out to launchctl is macOS-only.
// Same arrangement as schtasks.go, and for the same reason: the plist was the
// one installer nothing on the dev box could check.

const label = "net.stackdrift.watch"

func launchdArgs(plan Plan) []string {
	args := []string{plan.Exec, "watch"}
	if plan.Interval == config.IntervalRealtime {
		args = append(args, "--resident")
	}
	return append(args, ScheduledFlag)
}

// launchd runs the agent with no sandbox of its own, so unlike the systemd unit
// nothing here changes with the auto-update answer. The agent can already write
// wherever the user can.
func plistBody(plan Plan) string {
	realtime := plan.Interval == config.IntervalRealtime

	schedule := fmt.Sprintf("\t<key>StartInterval</key>\n\t<integer>%d</integer>\n",
		config.IntervalSeconds(plan.Interval))
	if realtime {
		// KeepAlive restarts the watcher if it dies, and RunAtLoad starts it
		// without waiting for the first interval that a resident process does
		// not have. It is also what lets a resident watcher replace its own
		// binary and stand down for launchd to start the new one.
		schedule = "\t<key>KeepAlive</key>\n\t<true/>\n"
	}

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	body.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	body.WriteString("\t<!-- stackdrift-interval=" + plan.Interval + " -->\n")
	body.WriteString("\t<key>Label</key>\n\t<string>" + label + "</string>\n")
	body.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range launchdArgs(plan) {
		body.WriteString("\t\t<string>" + escape(arg) + "</string>\n")
	}
	body.WriteString("\t</array>\n")
	body.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	body.WriteString(schedule)
	// Below normal priority, because nothing here is worth competing with what
	// the person at the keyboard is doing.
	body.WriteString("\t<key>Nice</key>\n\t<integer>10</integer>\n")
	body.WriteString("\t<key>LowPriorityIO</key>\n\t<true/>\n")
	body.WriteString("\t<key>ProcessType</key>\n\t<string>Background</string>\n")
	body.WriteString("</dict>\n</plist>\n")
	return body.String()
}

// The interval is read back out of the plist rather than trusted from the saved
// preferences, so status reports what is actually installed.
func intervalFromPlist(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "<!-- stackdrift-interval="); found {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "-->"))
		}
	}
	return ""
}

// execFromPlist is the first ProgramArguments entry, which is the binary
// launchd starts.
func execFromPlist(body string) string {
	inArgs := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "<key>ProgramArguments</key>" {
			inArgs = true
			continue
		}
		if !inArgs {
			continue
		}
		if rest, found := strings.CutPrefix(trimmed, "<string>"); found {
			return unescape(strings.TrimSuffix(rest, "</string>"))
		}
		if trimmed == "</array>" {
			return ""
		}
	}
	return ""
}

// xml.EscapeText emits numeric entities for tab, newline and carriage return as
// well as the named ones, so a table of names alone leaves them in the value and
// the path is then written back with the entity still in it.
func unescape(value string) string {
	named := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&#x9;", "\t", "&#xA;", "\n", "&#xD;", "\r")
	// Ampersand last, or an entity produced by an earlier replacement would be
	// decoded a second time
	return strings.ReplaceAll(named.Replace(value), "&amp;", "&")
}

func escape(value string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}
