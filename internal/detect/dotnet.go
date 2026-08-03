package detect

import (
	"regexp"
	"strings"
)

var (
	targetFrameworkRe = regexp.MustCompile(`(?i)<TargetFrameworks?>([^<]+)</TargetFrameworks?>`)
	// Classic projects, the ones that keep their packages in a packages.config,
	// declare the framework this way and never match the pattern above
	targetFrameworkVersionRe = regexp.MustCompile(`(?i)<TargetFrameworkVersion>\s*v?(\d+(?:\.\d+)*)\s*</TargetFrameworkVersion>`)
	netCoreRe                = regexp.MustCompile(`^net(\d+)\.(\d+)$`)
	netFrameworkRe           = regexp.MustCompile(`^net(\d)(\d)(\d)?$`)
)

func detectDotNet(result *Result, path string) {
	content, ok := readCapped(path)
	if !ok {
		return
	}

	if match := targetFrameworkRe.FindStringSubmatch(content); match != nil {
		for _, tfm := range strings.Split(match[1], ";") {
			tfm = strings.TrimSpace(strings.ToLower(tfm))
			if tfm == "" {
				continue
			}
			addFramework(result, moniker(tfm))
		}
		return
	}

	if match := targetFrameworkVersionRe.FindStringSubmatch(content); match != nil {
		result.Technologies = append(result.Technologies, Technology{
			Name:     ".NET Full Framework",
			Version:  match[1],
			Category: "Framework",
			Source:   "csproj TargetFrameworkVersion",
		})
	}
}

func moniker(tfm string) string {
	if dash := strings.IndexByte(tfm, '-'); dash >= 0 {
		return tfm[:dash]
	}
	return tfm
}

func addFramework(result *Result, tfm string) {
	if core := netCoreRe.FindStringSubmatch(tfm); core != nil {
		result.Technologies = append(result.Technologies, Technology{
			Name:     ".NET Core SDK",
			Version:  core[1] + "." + core[2],
			Category: "Framework",
			Source:   "csproj TargetFramework",
		})
		return
	}

	if fw := netFrameworkRe.FindStringSubmatch(tfm); fw != nil {
		version := fw[1] + "." + fw[2]
		if fw[3] != "" {
			version += "." + fw[3]
		}
		result.Technologies = append(result.Technologies, Technology{
			Name:     ".NET Full Framework",
			Version:  version,
			Category: "Framework",
			Source:   "csproj TargetFramework",
		})
	}
}
