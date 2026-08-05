//go:build windows

package detect

import (
	"context"
	"os/exec"
	"time"
)

// Windows Update answers only over COM, and one number is not worth a COM
// binding as a third dependency, so the query goes through the PowerShell that
// ships with every Windows install.
//
// Online is off on purpose. An online search asks Microsoft directly and can
// take minutes, while the watch service runs as often as every five minutes.
// Windows already searches on its own schedule, so the cached answer is the
// same answer without the traffic.
//
// MsrcSeverity is set only on updates the Microsoft Security Response Centre
// has rated, which is exactly the set worth counting on its own.
//
// The search date is read separately and printed even when it is blank, because
// blank is what tells the parser the cache has never been filled and a count of
// zero means nobody looked rather than nothing is due.
const windowsUpdateScript = `
$ErrorActionPreference = 'Stop'
$searcher = (New-Object -ComObject Microsoft.Update.Session).CreateUpdateSearcher()
$searcher.Online = $false
$found = $searcher.Search('IsInstalled=0 and IsHidden=0')
$security = 0
foreach ($update in $found.Updates) { if ($update.MsrcSeverity) { $security++ } }
$searched = ''
try {
  $last = (New-Object -ComObject Microsoft.Update.AutoUpdate).Results.LastSearchSuccessDate
  if ($last -is [datetime] -and $last.Year -gt 1900) { $searched = $last.ToUniversalTime().ToString('o') }
} catch { }
Write-Output "searched=$searched"
Write-Output "pending=$($found.Updates.Count)"
Write-Output "security=$security"
`

// The only command in the CLI with a deadline on it. Every other one runs a
// local tool that answers at once; this one talks to a service that can sit
// there, and a scan that never returns is worse than a scan that reports
// nothing. Generous rather than tight, because the cache is slow to open on a
// machine that has been up for months.
const windowsUpdateTimeout = 60 * time.Second

func pendingUpdates(onAsk func()) (UpdateStatus, bool) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		return UpdateStatus{}, false
	}

	if onAsk != nil {
		onAsk()
	}

	ctx, cancel := context.WithTimeout(context.Background(), windowsUpdateTimeout)
	defer cancel()

	// EncodedCommand rather than a quoted -Command string. PowerShell rebuilds
	// its own command line from the arguments Go hands it, so a script carrying
	// quotes and dollar signs arrives re-parsed; base64 has nothing to re-parse.
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodeCommand(windowsUpdateScript))

	// Without this the deadline only kills the process; Output goes on waiting
	// for the pipes, which anything powershell left holding keeps open. The
	// deadline has to be a bound on the call, not just on the child.
	cmd.WaitDelay = 5 * time.Second

	// Output rather than CombinedOutput, so a warning on the error stream cannot
	// land in the middle of the counts.
	output, err := cmd.Output()
	if err != nil {
		// Windows Update switched off, a policy that refuses the query, or a
		// machine locked down enough to refuse PowerShell. None of that is the
		// scan's to report, and all of it means nobody could ask.
		return UpdateStatus{}, false
	}
	return parseUpdateCounts(string(output))
}
