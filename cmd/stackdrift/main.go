package main

import (
	"fmt"
	"io"
	"os"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/commands"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

var version = "dev"

// check exits exitFailure when it finds an advisory, which is its contract in a
// pipeline, so a lapsed plan is given a code of its own. Sharing one would turn
// a build red for a billing reason and have it triaged as a security finding.
const (
	exitFailure    = 1
	exitPlanLapsed = 3
)

type command struct {
	name    string
	run     func([]string) error
	help    string
	options []commands.OptionInfo
	hidden  bool
}

func main() {
	// The server refuses a build behind the current release, so it has to know
	// which one is calling.
	api.Version = version
	os.Exit(run(os.Args[1:], registry(), os.Stdout, os.Stderr))
}

// commandList is the one place a command is declared. The usage text, the
// dispatch table and shell completion all read from it, so a new command shows
// up in all three without being listed anywhere else.
func commandList() []command {
	return []command{
		{name: "login", run: commands.Login, help: "sign in through the StackDrift website"},
		{name: "scan", run: commands.Scan, help: "detect technologies and dependencies (add --yes to accept all)",
			options: []commands.OptionInfo{{Name: "--yes", Help: "accept every detection without prompting"}}},
		{name: "status", run: commands.Status, help: "show tracked technologies and dependencies"},
		{name: "check", run: commands.Check, help: "report CVE status, exit non-zero if any are found"},
		{name: "remove", run: commands.Remove, help: "remove technologies or dependencies from this system"},
		{name: "service", run: commands.Service, help: "manage the background service that watches for stack changes",
			options: []commands.OptionInfo{
				{Name: "install", Help: "install the service and choose how often it checks"},
				{Name: "uninstall", Help: "remove the service"},
				{Name: "status", Help: "show whether the service is installed and running"},
				{Name: "--interval", Help: "realtime, 5m, hourly, twicedaily, daily or weekly"},
			}},
		{name: "watch", run: commands.Watch, help: "check now for stack changes and update StackDrift",
			options: []commands.OptionInfo{{Name: "--resident", Help: "stay running and watch continuously"}}},
		{name: "whoami", run: commands.Whoami, help: "show the signed in account"},
		{name: "logout", run: commands.Logout, help: "remove the saved credentials"},
		{name: "update", run: runUpdate, help: "download and install the latest release",
			options: []commands.OptionInfo{{Name: "--force", Help: "reinstall even when already up to date"}}},
		{name: "completion", run: runCompletion, help: "print a shell completion script",
			options: shellOptions()},
		{name: "version", run: showVersion, help: "print the CLI version"},
		{name: "__complete", run: runCompleteLine, hidden: true},
	}
}

func registry() map[string]command {
	out := make(map[string]command)
	for _, c := range commandList() {
		out[c.name] = c
	}
	return out
}

func run(args []string, registry map[string]command, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return exitFailure
	}

	name := args[0]
	if name == "help" || name == "-h" || name == "--help" {
		usage(stdout)
		return 0
	}

	cmd, ok := registry[name]
	if !ok {
		fmt.Fprintln(stderr, "unknown command: "+name)
		usage(stdout)
		return exitFailure
	}

	err := cmd.run(args[1:])

	// Being behind is the one failure the same binary can never retry its way
	// out of, so it updates itself and runs the command again. This stays ahead
	// of every other reading of the failure, so a build that is both stale and
	// locked out can still upgrade itself.
	if api.IsUpgradeRequired(err) && !alreadyUpgraded() {
		return upgradeAndRerun(args, stdout, stderr, err)
	}

	if commands.IsPlanRequired(err) {
		fmt.Fprintln(stderr, "error: "+err.Error())
		fmt.Fprintln(stderr, commands.PlanHint(err, config.BaseURL()))
		return exitPlanLapsed
	}

	// A token can be revoked between the startup check and any call that
	// follows it, so every command's failure goes through the same reading of
	// a rejection rather than reporting it as a plain request error.
	if err := commands.ExpireSession(err); err != nil {
		fmt.Fprintln(stderr, "error: "+err.Error())
		return exitFailure
	}
	return 0
}

func showVersion([]string) error {
	fmt.Printf("stackdrift %s (server %s)\n", version, config.BaseURL())
	return nil
}

func runUpdate(args []string) error {
	return commands.Update(version, args)
}

func runCompletion(args []string) error {
	return commands.Completion(os.Stdout, args)
}

func runCompleteLine(args []string) error {
	line := ""
	if len(args) > 0 {
		line = args[0]
	}
	return commands.CompleteLine(os.Stdout, completionInfo(), line)
}

func shellOptions() []commands.OptionInfo {
	var out []commands.OptionInfo
	for _, shell := range commands.CompletionShells() {
		out = append(out, commands.OptionInfo{Name: shell})
	}
	return out
}

func completionInfo() []commands.CommandInfo {
	var out []commands.CommandInfo
	for _, c := range commandList() {
		if c.hidden {
			continue
		}
		out = append(out, commands.CommandInfo{Name: c.name, Help: c.help, Options: c.options})
	}
	return out
}

func usage(out io.Writer) {
	fmt.Fprintln(out, "StackDrift CLI")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage: stackdrift <command>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	for _, c := range commandList() {
		if c.hidden {
			continue
		}
		fmt.Fprintf(out, "  %-11s %s\n", c.name, c.help)
	}
	// STACKDRIFT_URL still works and is deliberately not advertised here. It
	// points the CLI at staging, which is ours to test against rather than
	// something to invite users to reconfigure.
}
