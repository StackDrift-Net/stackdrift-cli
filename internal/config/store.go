package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StoreDir is where project links live. They are kept outside the scanned
// directory because a scan target is often a public web root, where the file
// would be readable by anyone who requests it.
//
// Every read of the store resolves its path through here, so this is also where
// a store path left holding a file is repaired.
func StoreDir() (string, error) {
	if _, err := reclaimStore(); err != nil {
		return "", err
	}
	return storePath()
}

// storePath answers where the store belongs without touching the disk, so the
// repair below can ask for the path it is about to repair.
func storePath() (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv("STACKDRIFT_HOME")); fromEnv != "" {
		return fromEnv, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".stackdrift"), nil
}

// reclaimStore turns a store path that holds a file back into a directory.
// Releases before v0.1.5 wrote the link into the directory being scanned, so a
// home directory scanned back then still has a ~/.stackdrift file sitting
// exactly where the store directory now goes, and every store read fails with
// "not a directory". The link that file holds is carried into the new layout
// and answered back, so a scan of the directory it describes can report the
// move rather than silently losing the tracked versions.
func reclaimStore() (*ProjectConfig, error) {
	store, err := storePath()
	if err != nil {
		return nil, err
	}

	// Stat rather than Lstat: a store symlinked to a directory elsewhere is a
	// deliberate setup and nothing here should disturb it. A path that cannot be
	// stat'd is left to the caller, whose own error says more than this one.
	info, err := os.Stat(store)
	if err != nil || info.IsDir() {
		return nil, nil
	}

	stranded, err := readProjectFile(store)
	if err != nil || stranded == nil || stranded.ProjectID <= 0 {
		// Whatever else is occupying the path is kept rather than deleted. It
		// sits in the user's home directory and this is its only chance.
		return nil, os.Rename(store, backupPath(store))
	}

	if err := os.Remove(store); err != nil {
		return nil, err
	}

	// The old layout put the link inside the directory it described, which for
	// this path is the directory the store now lives in.
	stranded.addPath(absolutePath(filepath.Dir(store)))

	path := filepath.Join(store, strconv.Itoa(stranded.ProjectID), ProjectFileName)
	if err := writeProjectFile(path, stranded); err != nil {
		return nil, err
	}
	return stranded, nil
}

// backupPath picks a name beside the store that is not already taken, so a
// second file stranded there cannot overwrite the first one kept aside.
func backupPath(store string) string {
	candidate := store + ".bak"
	for i := 2; i < 100; i++ {
		if _, err := os.Lstat(candidate); err != nil {
			break
		}
		candidate = store + ".bak." + strconv.Itoa(i)
	}
	return candidate
}
