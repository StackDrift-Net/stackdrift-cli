package commands

import (
	"sync"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

// How long an answer is held before the machine is asked again. Windows
// refreshes the cache this reads at most once a day, while a resident watcher
// comes round every ten seconds on a directory whose lock files keep moving, so
// asking per sweep would start a process a few hundred times an hour for a
// number that had not moved.
//
// It only bites in resident mode, which is one long lived process. The scheduled
// task starts fresh every run and is limited by its own interval, and an
// interactive scan is a new process every time, so somebody who asks by hand
// always gets a fresh answer.
const hostUpdateInterval = time.Hour

// The seam under the cache. readHostUpdates in scan.go is the seam above it, and
// the two are separate on purpose: one proves a sweep asks once for all its
// systems, this one proves the answer is not asked for again straight away.
var (
	hostUpdateMu      sync.Mutex
	hostUpdateAskedAt time.Time
	hostUpdateCached  api.ScanReportRequest
	hostUpdateClock   = time.Now
	hostUpdateProbe   = detect.PendingUpdates
)

// hostUpdates asks the machine what its own updater is holding, at most once
// every hostUpdateInterval.
//
// Nobody having been able to ask leaves both counts nil, which is the whole
// point of them being pointers. Sending zero would tell the website the machine
// is up to date on the strength of never having looked. A failed answer is held
// as long as a good one: a machine that cannot answer will not start being able
// to ten seconds later, and the server leaves the stored counts alone either way.
func hostUpdates(announce func()) api.ScanReportRequest {
	hostUpdateMu.Lock()
	defer hostUpdateMu.Unlock()

	now := hostUpdateClock()
	if !hostUpdateAskedAt.IsZero() && now.Sub(hostUpdateAskedAt) < hostUpdateInterval {
		return hostUpdateCached
	}

	hostUpdateAskedAt = now
	hostUpdateCached = api.ScanReportRequest{}
	if status, ok := hostUpdateProbe(announce); ok {
		hostUpdateCached = api.ScanReportRequest{PendingUpdates: &status.Pending, SecurityUpdates: &status.Security}
	}
	return hostUpdateCached
}

// scanHostUpdates is the interactive form. Somebody is watching, and the query
// talks to a service that can sit there, so it says what it is waiting on.
func scanHostUpdates() api.ScanReportRequest {
	return readHostUpdates(func() {
		ui.Println("Checking for operating system updates ...")
	})
}

// printPendingUpdates says what the machine's own updater is holding. Nothing
// waiting says nothing at all: a scan is about the stack, and a line confirming
// the machine is current on every single run is noise.
func printPendingUpdates(updates api.ScanReportRequest) {
	if updates.PendingUpdates == nil || *updates.PendingUpdates == 0 {
		return
	}
	if updates.SecurityUpdates != nil && *updates.SecurityUpdates > 0 {
		ui.Printf("Operating system updates waiting: %d (%d security).\n",
			*updates.PendingUpdates, *updates.SecurityUpdates)
		return
	}
	ui.Printf("Operating system updates waiting: %d.\n", *updates.PendingUpdates)
}
