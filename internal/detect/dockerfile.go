package detect

import (
	"bufio"
	"regexp"
	"strings"
)

var fromRe = regexp.MustCompile(`(?i)^\s*FROM\s+(\S+)`)
var leadingIntRe = regexp.MustCompile(`^\d+`)

// The whole numeric part of a tag, so a tag pinned to a patch keeps it. The
// suffix a tag carries after it, such as -alpine, names the base image rather
// than the release.
var imageNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)*`)

var dockerImageDistros = map[string]string{
	"ubuntu": "Ubuntu",
	"debian": "Debian",
	"fedora": "Fedora",
	"alpine": "Alpine Linux",
}

func detectDockerfile(result *Result, path string) {
	content, ok := readCapped(path)
	if !ok {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		match := fromRe.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		detectDockerImage(result, match[1])
	}
}

func detectDockerImage(result *Result, image string) {
	image = strings.TrimPrefix(image, "docker.io/library/")

	repo, tag, _ := strings.Cut(image, ":")

	if name, isDotNet := dotNetImageTechnology(repo); isDotNet {
		addDotNetImage(result, name, tag)
		return
	}

	name, known := dockerImageDistros[lastSegment(repo)]
	if !known {
		return
	}

	result.Technologies = append(result.Technologies, Technology{
		Name:     name,
		Version:  imageVersion(tag),
		Category: "OperatingSystem",
		Source:   "Dockerfile",
	})
}

// dotNetImageTechnology says which catalog entry a dotnet image belongs to. The
// images are split by what they hold: dotnet/sdk builds, while dotnet/aspnet
// and dotnet/runtime only run, and the two are patched and advised separately.
// The framework images are Windows containers carrying Full Framework, which is
// a different technology again.
func dotNetImageTechnology(repo string) (string, bool) {
	if !strings.Contains(repo, "dotnet/") {
		return "", false
	}
	if strings.Contains(repo, "dotnet/framework/") {
		return ".NET Full Framework", true
	}
	if lastSegment(repo) == "sdk" {
		return ".NET Core SDK", true
	}
	return ".NET Core Runtime", true
}

// addDotNetImage keeps the build when the tag pins one. A tag naming only the
// line is left without a build, which is the honest reading: the image floats
// onto whatever patch is current.
func addDotNetImage(result *Result, name, tag string) {
	version := imageNumberRe.FindString(tag)
	if version == "" {
		return
	}

	line, build := version, ""
	// Full Framework releases are the line, so 4.8.1 is a release rather than a
	// build of 4.8, and splitting it would track a version nobody is on.
	if name != ".NET Full Framework" {
		line = dotNetLine(version)
		if line != version {
			build = version
		}
	}

	result.Technologies = append(result.Technologies, Technology{
		Name:     name,
		Version:  line,
		Kernel:   build,
		Category: "Framework",
		Source:   "Dockerfile",
	})
}

func lastSegment(repo string) string {
	if slash := strings.LastIndex(repo, "/"); slash >= 0 {
		return repo[slash+1:]
	}
	return repo
}

func imageVersion(tag string) string {
	if match := kernelVersionRe.FindString(tag); match != "" {
		return match
	}
	if match := leadingIntRe.FindString(tag); match != "" {
		return match
	}
	return ""
}
