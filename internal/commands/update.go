package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releaseRepo = "StackDrift-Net/stackdrift-cli"

func Update(current string, args []string) error {
	force := false
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		default:
			return fmt.Errorf("unknown option: %s", a)
		}
	}

	apiBase := updateBase("STACKDRIFT_UPDATE_API", "https://api.github.com")
	downloadBase := updateBase("STACKDRIFT_UPDATE_DOWNLOAD", "https://github.com")

	latest, err := fetchLatestTag(apiBase, releaseRepo)
	if err != nil {
		return err
	}

	if !force && !needsUpdate(current, latest) {
		fmt.Printf("Already on the latest version (%s).\n", latest)
		return nil
	}

	asset := assetName(runtime.GOOS, runtime.GOARCH)
	url := downloadURL(downloadBase, releaseRepo, asset)

	fmt.Printf("Downloading %s ...\n", asset)
	tmp, err := downloadBinary(url)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	if err := replaceRunning(tmp); err != nil {
		return err
	}

	fmt.Printf("Updated to %s.\n", latest)
	return nil
}

func updateBase(key, fallback string) string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); v != "" {
		return v
	}
	return fallback
}

func assetName(goos, goarch string) string {
	name := fmt.Sprintf("stackdrift-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func downloadURL(base, repo, asset string) string {
	return fmt.Sprintf("%s/%s/releases/latest/download/%s", base, repo, asset)
}

func needsUpdate(current, latest string) bool {
	c := normalizeVersion(current)
	if c == "" || c == "dev" {
		return true
	}
	return c != normalizeVersion(latest)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func fetchLatestTag(apiBase, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("no published release found (the repository may be private or have no releases yet)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("could not check for updates (status %d)", resp.StatusCode)
	}
	return parseLatestTag(data)
}

func parseLatestTag(data []byte) (string, error) {
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", errors.New("the release feed did not include a version tag")
	}
	return release.TagName, nil
}

func downloadBinary(url string) (string, error) {
	exe, err := currentExecutable()
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no binary was published for this platform (%s)", filepath.Base(url))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download failed (status %d)", resp.StatusCode)
	}

	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".stackdrift-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot write to %s (need write access to update in place): %w", dir, err)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

func replaceRunning(newPath string) error {
	exe, err := currentExecutable()
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(newPath, 0o755); err != nil {
			return err
		}
		if err := os.Rename(newPath, exe); err != nil {
			return fmt.Errorf("could not replace %s: %w", exe, err)
		}
		return nil
	}

	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("could not replace %s: %w", exe, err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Rename(old, exe)
		return fmt.Errorf("could not replace %s: %w", exe, err)
	}
	os.Remove(old)
	return nil
}
