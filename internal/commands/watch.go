package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/detect"
	"github.com/digitalaffinity-au/stackdrift-cli/internal/ui"
)

// How often the resident watcher stats the files it is watching, and how long
// it will go without a full walk. The sweep is a handful of stat calls and
// costs nothing measurable; the full walk is what finds software that was
// installed somewhere the watch set does not reach.
const (
	sweepInterval   = 10 * time.Second
	fullScanEvery   = time.Hour
	backoffOnError  = 5 * time.Minute
	settleAfterMove = 2 * time.Second
)

func Watch(args []string) error {
	if hasFlag(args, "--resident", "-r") {
		return watchResident()
	}
	return watchOnce()
}

func watchOnce() error {
	result, err := runCycle()
	if err != nil {
		return err
	}
	if result.Changed > 0 {
		ui.Printf("Updated StackDrift: %d change(s).\n", result.Changed)
	}
	return nil
}

// watchResident is the near realtime mode. It holds no scan results between
// sweeps and hands memory back to the OS after each one, so what stays resident
// is the Go runtime and a list of file paths.
func watchResident() error {
	ui.Println("Watching for stack changes. Sweeping every " + sweepInterval.String() + ".")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	watching, lastFull := firstSweep()

	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			ui.Println("Stopped watching.")
			return nil
		case <-ticker.C:
			due := time.Since(lastFull) >= fullScanEvery
			if !due && !moved(watching) {
				continue
			}
			if !due {
				// A package manager rewrites a lock file in several steps, and
				// scanning between two of them reads a half written file.
				time.Sleep(settleAfterMove)
			}

			result, err := runCycle()

			// Re-stamped whether the sweep worked or not. Leaving the old
			// stamps in place after a failure would leave the file that woke us
			// still reading as moved, so the next tick ten seconds later would
			// retry, and so would every tick after it for as long as the
			// failure lasted. The backoff below is what governs the retry.
			watching = restamp(watching, result.Watching)

			if err != nil {
				ui.Println("Watch sweep failed: " + err.Error())
				lastFull = time.Now().Add(backoffOnError - fullScanEvery)
				continue
			}

			lastFull = time.Now()
			// The scan holds every manifest it read, which is the high water
			// mark of this process. Handing it back keeps a watcher that runs
			// for months sitting at what it started with.
			debug.FreeOSMemory()
		}
	}
}

func firstSweep() (map[string]string, time.Time) {
	result, err := runCycle()
	if err != nil {
		ui.Println("First sweep failed: " + err.Error())
		return map[string]string{}, time.Now().Add(backoffOnError - fullScanEvery)
	}
	debug.FreeOSMemory()
	return fingerprint(result.Watching), time.Now()
}

// moved reports whether any watched file has changed size, changed timestamp,
// appeared or disappeared. Stat is used rather than reading the files, so the
// sweep costs the same whether it is watching a 40 byte version file or a 12 MB
// lock file.
func moved(watching map[string]string) bool {
	for path, was := range watching {
		if stamp(path) != was {
			return true
		}
	}
	return false
}

// restamp takes the freshly discovered watch set when there is one, and falls
// back to re-reading the paths already being watched when a sweep failed before
// it could report any.
func restamp(previous map[string]string, discovered []string) map[string]string {
	if len(discovered) > 0 {
		return fingerprint(discovered)
	}

	paths := make([]string, 0, len(previous))
	for path := range previous {
		paths = append(paths, path)
	}
	return fingerprint(paths)
}

func fingerprint(paths []string) map[string]string {
	sort.Strings(paths)
	out := make(map[string]string, len(paths))
	for _, path := range paths {
		out[path] = stamp(path)
	}
	return out
}

// A missing file gets a stamp of its own rather than an empty one, so that a
// file which does not exist yet and a file that has just been deleted are told
// apart from each other and from an unreadable one.
func stamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	return strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
}

// readable reports whether a scanned directory can actually be walked. The
// scan itself cannot answer this: its walk swallows every error including the
// one for the root, so a directory on an unmounted volume comes back as a
// successful scan of a machine with nothing in it. Nothing may be retired on
// that, so the caller has to know the difference.
func readable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	// Stat succeeds on a directory the process cannot list, which is the case a
	// service running as a different user hits, so the open is the real test.
	handle, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer handle.Close()

	_, err = handle.Readdirnames(1)
	return err == nil || err == io.EOF
}

// watchPaths is the curated set a sweep stats: every manifest the scan read,
// every technology whose source names a real file, and the two host files that
// change when the machine is upgraded underneath us.
func watchPaths(result *detect.Result) []string {
	seen := map[string]bool{}
	var paths []string

	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		if _, err := os.Stat(path); err != nil {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}

	for _, m := range result.Manifests {
		add(m.Path)
	}
	for _, t := range result.Technologies {
		add(sourcePath(t.Source))
	}

	add(detect.SourceOsRelease)
	// The running kernel is not a file, but the string that names it is, and it
	// changes the moment a new kernel is booted.
	add("/proc/version")
	return paths
}

func sourcePath(source string) string {
	if source == detect.SourceOsRelease {
		return source
	}
	if rest, found := cutHostPrefix(source); found {
		return rest
	}
	if len(source) > 0 && source[0] == '/' {
		return source
	}
	return ""
}

func cutHostPrefix(source string) (string, bool) {
	if len(source) < len(detect.SourceHostPrefix) {
		return "", false
	}
	if source[:len(detect.SourceHostPrefix)] != detect.SourceHostPrefix {
		return "", false
	}
	return source[len(detect.SourceHostPrefix):], true
}

func manifestDigest(bundle []detect.Manifest) string {
	names := make([]string, 0, len(bundle))
	contents := map[string]string{}
	for _, m := range bundle {
		names = append(names, m.FileName)
		contents[m.FileName] = m.Content
	}
	// Sorted, because the order the walk happens to return files in is not a
	// change to the files themselves.
	sort.Strings(names)

	sum := sha256.New()
	for _, name := range names {
		sum.Write([]byte(name))
		sum.Write([]byte{0})
		sum.Write([]byte(contents[name]))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func storedDigest(cfg *config.ProjectConfig, groupName string) string {
	for _, group := range cfg.DependencyGrp {
		if group.Name == groupName {
			return group.Digest
		}
	}
	return ""
}

func setDigest(cfg *config.ProjectConfig, groupName, digest string) {
	for i := range cfg.DependencyGrp {
		if cfg.DependencyGrp[i].Name == groupName {
			cfg.DependencyGrp[i].Digest = digest
			return
		}
	}
}
