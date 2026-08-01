package commands

import (
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

// One service covers every directory scanned on the machine, so a machine that
// already has one has answered the offer. Without this the preferences file
// being lost would put the question again, and answering no would print "Not
// installed" over a service that is still running.
func TestAdoptInstalledService_ServiceAlreadyInstalled_RecordsTheMachineAsAnswered(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	adoptInstalledService(service.State{Installed: true, Interval: config.IntervalWeekly})

	settings := config.LoadWatch()
	if !settings.Asked {
		t.Fatal("a machine with a service installed must not be offered another")
	}
	if !settings.Enabled {
		t.Fatal("expected the installed service to be recorded as enabled")
	}
	if settings.Interval != config.IntervalWeekly {
		t.Fatalf("expected the interval the unit is running on, got %q", settings.Interval)
	}
}

// An older unit, or one edited by hand, may carry no interval marker. The
// answer still has to be recorded, or the offer returns after every scan.
func TestAdoptInstalledService_UnitWithNoInterval_StillRecordsTheAnswer(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	adoptInstalledService(service.State{Installed: true})

	if settings := config.LoadWatch(); !settings.Asked || !settings.Enabled {
		t.Fatalf("expected the machine recorded as answered, got %+v", settings)
	}
}
