package detect

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxManifestBytes = 5 * 1024 * 1024

func scanTree(root string, result *Result) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") || isWordPressUploads(path) || isVendorDir(path)) {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		lower := strings.ToLower(name)

		switch {
		case lower == "package.json":
			addManifest(result, "Npm", path, name, true)
		case lower == "package-lock.json":
			addManifest(result, "Npm", path, name, false)
		case lower == "yarn.lock":
			addManifest(result, "Npm", path, name, false)
		case lower == "pnpm-lock.yaml":
			addManifest(result, "Npm", path, name, false)
		case lower == "packages.lock.json":
			addManifest(result, "NuGet", path, name, false)
		// Old style .NET Framework projects keep their packages here rather
		// than in the csproj, which lists only assembly references.
		case lower == "packages.config":
			addManifest(result, "NuGet", path, name, false)
		case lower == "directory.packages.props":
			addManifest(result, "NuGet", path, name, false)
		case strings.HasSuffix(lower, ".csproj"):
			addManifest(result, "NuGet", path, name, true)
			detectDotNet(result, path)
		case lower == "global.json":
			detectGlobalJson(result, path)
		case lower == "composer.json":
			detectLaravel(result, path)
		case lower == "dockerfile" || strings.HasPrefix(lower, "dockerfile."):
			detectDockerfile(result, path)
		case isWordPressVersionFile(path, name):
			detectWordPress(result, root, path)
		}
		return nil
	})
}

func addManifest(result *Result, ecosystem, path, name string, primary bool) {
	content, ok := readCapped(path)
	if !ok {
		return
	}
	result.Manifests = append(result.Manifests, Manifest{
		Ecosystem: ecosystem,
		Path:      path,
		FileName:  name,
		Content:   content,
		Primary:   primary,
	})
}

func readCapped(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxManifestBytes {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}
