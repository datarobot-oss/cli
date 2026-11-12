// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/datarobot/cli/internal/version"
)

// currentCLIVersion is a package-level seam for the running CLI version.
// It defaults to version.Version, but tests MUST override it (with a
// t.Cleanup/defer restore) rather than relying on the real value: in `go
// test`, version.Version is the literal "dev", which is an unparseable-CLI
// bypass under compatibleCLIVersion and would silently make every bound
// assertion pass regardless of the production logic.
var currentCLIVersion = version.Version

// coreVersion strips any prerelease/build metadata from v, returning a
// version containing only major.minor.patch. This keeps comparisons
// symmetric: without it, a prerelease running CLI (e.g. 1.2.0-rc.1) would
// compare as less than its own release (1.2.0) under semver ordering, so it
// would fail a maxCLIVersion bound equal to its own release.
func coreVersion(v *semver.Version) *semver.Version {
	return semver.New(v.Major(), v.Minor(), v.Patch(), "", "")
}

// compatibleCLIVersion reports whether cliVersion satisfies the inclusive
// [minBound, maxBound] range. Either bound may be empty, meaning
// unconstrained on that side. Comparison uses core versions (major.minor.
// patch) only, so a prerelease CLI version is judged by its release version.
//
// A malformed minBound or maxBound ALWAYS causes a skip (returns false with
// a non-nil error) — this is checked before the CLI version is even parsed,
// so it takes precedence over the dev/unparseable-CLI-version bypass below.
//
// An unparseable cliVersion (including the default "dev" build) is treated
// as a bypass: once the bounds themselves are confirmed well-formed, the
// plugin loads unconditionally, since there is no reliable CLI version to
// compare against.
func compatibleCLIVersion(cliVersion, minBound, maxBound string) (bool, error) {
	if minBound == "" && maxBound == "" {
		return true, nil
	}

	minVer, err := parseCLIVersionBound("minCLIVersion", minBound)
	if err != nil {
		return false, err
	}

	maxVer, err := parseCLIVersionBound("maxCLIVersion", maxBound)
	if err != nil {
		return false, err
	}

	cli, err := semver.NewVersion(cliVersion)
	if err != nil {
		// Unparseable/dev running CLI version: bypass now that the declared
		// bounds are confirmed well-formed.
		return true, nil
	}

	return versionWithinBounds(cli, minVer, maxVer), nil
}

// parseCLIVersionBound parses a declared minCLIVersion/maxCLIVersion value.
// An empty value is unconstrained (nil, nil). field names the manifest
// field in the returned error, for callers to surface an actionable message.
func parseCLIVersionBound(field, value string) (*semver.Version, error) {
	if value == "" {
		return nil, nil
	}

	v, err := semver.NewVersion(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}

	return v, nil
}

// versionWithinBounds reports whether cli's core version falls within the
// inclusive [minVer, maxVer] range. Either bound may be nil, meaning
// unconstrained on that side.
func versionWithinBounds(cli, minVer, maxVer *semver.Version) bool {
	core := coreVersion(cli)

	if minVer != nil && core.LessThan(coreVersion(minVer)) {
		return false
	}

	if maxVer != nil && core.GreaterThan(coreVersion(maxVer)) {
		return false
	}

	return true
}

// cliVersionSkip evaluates manifest's declared CLI version bounds against
// currentCLIVersion and returns a PluginConflict describing the skip when
// the plugin is not compatible, or nil when it may load. path identifies the
// executable (or managed plugin dir) being evaluated, for reporting.
func cliVersionSkip(manifest *PluginManifest, path string) *PluginConflict {
	ok, err := compatibleCLIVersion(currentCLIVersion, manifest.MinCLIVersion, manifest.MaxCLIVersion)
	if err == nil && ok {
		return nil
	}

	var detail string

	switch {
	case err != nil:
		detail = err.Error()
	case manifest.MinCLIVersion != "" && manifest.MaxCLIVersion != "":
		// Both bounds declared and the combined check failed: report the
		// bound that actually breached, preferring the minimum since a
		// version cannot violate both an inclusive min and an inclusive max
		// unless the manifest's own range is inverted.
		minOK, _ := compatibleCLIVersion(currentCLIVersion, manifest.MinCLIVersion, "")
		if !minOK {
			detail = fmt.Sprintf(
				"requires dr >= %s (running %s); run 'dr self update'",
				manifest.MinCLIVersion, currentCLIVersion,
			)
		} else {
			detail = fmt.Sprintf(
				"supports dr <= %s (running %s); update the plugin",
				manifest.MaxCLIVersion, currentCLIVersion,
			)
		}
	case manifest.MinCLIVersion != "":
		detail = fmt.Sprintf(
			"requires dr >= %s (running %s); run 'dr self update'",
			manifest.MinCLIVersion, currentCLIVersion,
		)
	default:
		detail = fmt.Sprintf(
			"supports dr <= %s (running %s); update the plugin",
			manifest.MaxCLIVersion, currentCLIVersion,
		)
	}

	return &PluginConflict{
		Name:   manifest.Name,
		Path:   path,
		Reason: SkipReasonVersionIncompatible,
		Detail: detail,
	}
}
