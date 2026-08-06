package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const WatchFileName = "watch.json"

// Intervals the background service can run at. Realtime is the only one that
// stays resident; the rest are handed to the platform's own scheduler, so
// nothing of ours is running between two of its runs.
//
// Only three of these are offered when installing. The rest stay recognised
// because installs in the field already run on them and because --interval
// still takes them, and a value the CLI no longer understands would report as
// no interval at all in status.
const (
	IntervalRealtime      = "realtime"
	IntervalFiveMin       = "5m"
	IntervalHourly        = "hourly"
	IntervalTwiceDay      = "twicedaily"
	IntervalDaily         = "daily"
	IntervalEveryOtherDay = "everyotherday"
	IntervalWeekly        = "weekly"
)

// Seconds each interval waits between full scans. Realtime carries the gap
// between two cheap stat sweeps, not between scans, which is why it is the one
// value that does not describe a scan.
func IntervalSeconds(interval string) int {
	switch interval {
	case IntervalRealtime:
		return 10
	case IntervalFiveMin:
		return 300
	case IntervalHourly:
		return 3600
	case IntervalTwiceDay:
		return 43200
	case IntervalDaily:
		return 86400
	case IntervalEveryOtherDay:
		return 172800
	case IntervalWeekly:
		return 604800
	default:
		return 0
	}
}

func IntervalLabel(interval string) string {
	switch interval {
	case IntervalRealtime:
		return "near realtime"
	case IntervalFiveMin:
		return "every 5 minutes"
	case IntervalHourly:
		return "hourly"
	case IntervalTwiceDay:
		return "twice a day"
	case IntervalDaily:
		return "daily"
	case IntervalEveryOtherDay:
		return "every other day"
	case IntervalWeekly:
		return "weekly"
	default:
		return interval
	}
}

func KnownInterval(interval string) bool {
	return IntervalSeconds(interval) > 0
}

// WatchSettings is answered once per machine rather than once per project,
// because one service covers every directory that has been scanned. Asked is
// what stops a declined offer being put again after every scan; it is the whole
// reason this file exists separately from whether the service is installed.
type WatchSettings struct {
	Version  int    `json:"version"`
	Asked    bool   `json:"asked"`
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval,omitempty"`

	// A pointer so "never answered" is a different thing from "answered no".
	// Every install that predates the question reads as nil, and nothing may
	// start replacing a binary on a machine whose owner was never asked.
	AutoUpdate *bool `json:"autoUpdate,omitempty"`

	// RFC 3339 UTC. What the check is throttled against.
	LastUpdateAt string `json:"lastUpdateCheck,omitempty"`

	// The version last written over the executable, which is NOT the version
	// this process is running. A watcher that cannot restart itself keeps
	// reporting the old one, and without this it would fetch the same release
	// every day for ever.
	UpdatedTo string `json:"updatedTo,omitempty"`

	// The binary the scheduler was pointed at. Read back when a setting is
	// changed, so a second copy of the CLI cannot quietly repoint the schedule
	// at itself. The installed entry is asked first, and this is what answers
	// on Windows, where schtasks localizes the field it would be read from.
	ServiceExec string `json:"serviceExec,omitempty"`

	// Why the binary cannot be replaced, when that has been established. Set
	// once and then checked, so a permanently unwritable install stops calling
	// out on every run.
	UpdateBlocked string `json:"updateBlocked,omitempty"`
}

func (s *WatchSettings) AutoUpdateEnabled() bool {
	return s != nil && s.AutoUpdate != nil && *s.AutoUpdate
}

func (s *WatchSettings) AutoUpdateAnswered() bool {
	return s != nil && s.AutoUpdate != nil
}

func WatchFilePath() (string, error) {
	store, err := StoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(store, WatchFileName), nil
}

// LoadWatch never fails on a damaged file. The only thing it decides is whether
// to put a question, and refusing to scan because a preferences file will not
// parse would be a worse answer than asking once more.
func LoadWatch() *WatchSettings {
	path, err := WatchFilePath()
	if err != nil {
		return &WatchSettings{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &WatchSettings{}
	}

	var settings WatchSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return &WatchSettings{}
	}
	return &settings
}

func SaveWatch(settings *WatchSettings) error {
	path, err := WatchFilePath()
	if err != nil {
		return err
	}
	if settings.Version == 0 {
		settings.Version = 1
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// Written to a temp file in the same directory and renamed, so a machine
	// that loses power mid-write is left with the old answers rather than half
	// a file that LoadWatch would read as never having been asked.
	temp, err := os.CreateTemp(filepath.Dir(path), ".watch-*")
	if err != nil {
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		os.Remove(temp.Name())
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		os.Remove(temp.Name())
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		os.Remove(temp.Name())
		return err
	}
	return nil
}

// UpdateWatch reads, changes and writes back, so a caller touches only the
// field it means to.
//
// Every writer used to pass a whole fresh struct literal. That was safe while
// there were three fields all decided at once, and stopped being safe the
// moment a background sweep started recording when it last checked for an
// update. An uninstall would have wiped that, and recording it would have wiped
// the interval.
func UpdateWatch(mutate func(*WatchSettings)) error {
	settings := LoadWatch()
	mutate(settings)
	return SaveWatch(settings)
}

// LinkedProjects reads every stored project link. The service scans all of them
// on one schedule, so a second project does not mean a second service.
func LinkedProjects() ([]*ProjectConfig, error) {
	store, err := StoreDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(store)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []*ProjectConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cfg, err := readProjectFile(filepath.Join(store, entry.Name(), ProjectFileName))
		if err != nil || cfg == nil || cfg.ProjectID <= 0 {
			continue
		}
		projects = append(projects, cfg)
	}
	return projects, nil
}
