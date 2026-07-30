package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const ProjectFileName = ".stackdrift"

type TrackedTechnology struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Category string `json:"category"`
	ID       int    `json:"id,omitempty"`
}

type TrackedDependencyGroup struct {
	Name      string   `json:"name"`
	Ecosystem string   `json:"ecosystem"`
	Manifests []string `json:"manifests"`

	// Digest of the file contents last uploaded for this group. The watcher
	// compares against it to decide whether a lock file has actually moved,
	// rather than re-uploading every manifest on every sweep.
	Digest string `json:"digest,omitempty"`
}

type ProjectConfig struct {
	Version       int                      `json:"version"`
	ProjectID     int                      `json:"projectId"`
	ProjectName   string                   `json:"projectName"`
	Paths         []string                 `json:"paths"`
	Technologies  []TrackedTechnology      `json:"technologies"`
	DependencyGrp []TrackedDependencyGroup `json:"dependencyGroups"`

	// Set when the link was just moved out of a scanned directory, so the
	// caller can tell the user the old file is gone.
	Migrated bool `json:"-"`
}

// StoreDir is where project links live. They are kept outside the scanned
// directory because a scan target is often a public web root, where the file
// would be readable by anyone who requests it.
func StoreDir() (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv("STACKDRIFT_HOME")); fromEnv != "" {
		return fromEnv, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".stackdrift"), nil
}

func ProjectFilePath(projectID int) (string, error) {
	store, err := StoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(store, strconv.Itoa(projectID), ProjectFileName), nil
}

func LegacyProjectFilePath(dir string) string {
	return filepath.Join(dir, ProjectFileName)
}

func LoadProject(dir string) (*ProjectConfig, error) {
	dir = absolutePath(dir)

	cfg, err := findLinked(dir)
	if err != nil || cfg != nil {
		return cfg, err
	}

	return migrateLegacy(dir)
}

func SaveProject(dir string, cfg *ProjectConfig) error {
	if cfg.ProjectID <= 0 {
		return errors.New("cannot save a project link without a project id")
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Technologies == nil {
		cfg.Technologies = []TrackedTechnology{}
	}
	if cfg.DependencyGrp == nil {
		cfg.DependencyGrp = []TrackedDependencyGroup{}
	}
	dir = absolutePath(dir)

	path, err := ProjectFilePath(cfg.ProjectID)
	if err != nil {
		return err
	}

	// Scanning a second directory for an existing project arrives here with a
	// freshly built config, so the directories already linked to that project
	// are merged back in rather than overwritten.
	if stored, err := readProjectFile(path); err == nil && stored != nil {
		for _, existing := range stored.Paths {
			cfg.addPath(existing)
		}
	}
	cfg.addPath(dir)

	// A directory belongs to one project, so claiming it here releases it from
	// whichever project held it before. Without this a directory reassigned to
	// a new project keeps resolving to the old one.
	if err := releasePath(dir, cfg.ProjectID); err != nil {
		return err
	}

	return writeProjectFile(path, cfg)
}

func writeProjectFile(path string, cfg *ProjectConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// The link names a project and every technology version tracked for it, so
	// it stays readable only by the user who scanned.
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// findLinked resolves a directory to its project by reading every stored link,
// since the store is keyed by project id and a project can be scanned from
// more than one directory.
func findLinked(dir string) (*ProjectConfig, error) {
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

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// A link that will not parse is skipped rather than failing every
		// other project. The next scan rewrites it.
		cfg, err := readProjectFile(filepath.Join(store, entry.Name(), ProjectFileName))
		if err != nil || cfg == nil {
			continue
		}
		if cfg.linkedTo(dir) {
			return cfg, nil
		}
	}
	return nil, nil
}

// releasePath drops dir from every stored project except keepID, removing a
// link entirely once it tracks no directories.
func releasePath(dir string, keepID int) error {
	store, err := StoreDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(store)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == strconv.Itoa(keepID) {
			continue
		}

		linkDir := filepath.Join(store, entry.Name())
		cfg, err := readProjectFile(filepath.Join(linkDir, ProjectFileName))
		if err != nil || cfg == nil || !cfg.linkedTo(dir) {
			continue
		}

		cfg.removePath(dir)
		if len(cfg.Paths) == 0 {
			if err := os.RemoveAll(linkDir); err != nil {
				return err
			}
			continue
		}
		if err := writeProjectFile(filepath.Join(linkDir, ProjectFileName), cfg); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacy(dir string) (*ProjectConfig, error) {
	legacy := LegacyProjectFilePath(dir)

	cfg, err := readProjectFile(legacy)
	if err != nil || cfg == nil {
		return nil, err
	}
	if cfg.ProjectID <= 0 {
		return nil, nil
	}

	if err := SaveProject(dir, cfg); err != nil {
		return nil, err
	}
	if err := os.Remove(legacy); err != nil {
		return nil, err
	}

	cfg.Migrated = true
	return cfg, nil
}

func readProjectFile(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *ProjectConfig) linkedTo(dir string) bool {
	for _, path := range c.Paths {
		if samePath(path, dir) {
			return true
		}
	}
	return false
}

func (c *ProjectConfig) addPath(dir string) {
	if c.linkedTo(dir) {
		return
	}
	c.Paths = append(c.Paths, dir)
}

func (c *ProjectConfig) removePath(dir string) {
	kept := c.Paths[:0]
	for _, path := range c.Paths {
		if !samePath(path, dir) {
			kept = append(kept, path)
		}
	}
	c.Paths = kept
}

func absolutePath(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(dir)
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
