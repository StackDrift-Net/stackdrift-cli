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
	return os.WriteFile(path, append(data, '\n'), 0o600)
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
