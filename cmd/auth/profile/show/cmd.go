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

package show

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// defaultProfileLabel is how the top-level (unnamed) profile is addressed
// on the command line and in output.
const defaultProfileLabel = config.DefaultProfileLabel

// profileDetail is the JSON representation of a single profile's resolved
// settings for --output-format json.
type profileDetail struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Endpoint    string `json:"endpoint"`
	EndpointOwn bool   `json:"endpoint_own"`
	HasToken    bool   `json:"has_token"`
	HasTokenOwn bool   `json:"has_token_own"`
	CACert      string `json:"ca_cert,omitempty"`
	CACertOwn   bool   `json:"ca_cert_own"`
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "🔍 Show a profile's resolved settings",
		Long: `Show a profile's resolved settings: its own endpoint/token/ca-cert,
falling back to the default profile's values for anything it doesn't define.

Defaults to the active profile (selected via --profile or
DATAROBOT_CLI_PROFILE) when no name is given. Pass "default" to see the
explicit profiles.default section when one is defined, or the top-level
(root) profile otherwise. The token itself is never printed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runShow,
	}

	return cmd
}

func runShow(cmd *cobra.Command, args []string) error {
	activeProfile := config.ActiveProfile()

	def, profiles, err := config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("failed to read profiles: %w", err)
	}

	// name is empty-string-means-root throughout, matching
	// config.ActiveProfile()'s convention; only displayName is user-facing.
	name := activeProfile
	if len(args) > 0 {
		name = config.NormalizeProfileName(args[0])

		// Positional compatibility: `auth profile show default` shows the
		// explicit profiles.default section when it is defined, and falls
		// back to the root-level profile when it is not. Flag/env selection
		// never falls back like this: an undefined selection is rejected by
		// config initialization before the command runs.
		if name == defaultProfileLabel && !profileListed(profiles, name) {
			name = ""
		}
	}

	own, err := resolveOwnSection(name, def, profiles)
	if err != nil {
		return err
	}

	displayName := name
	if displayName == "" {
		displayName = defaultProfileLabel
	}

	detail := resolveDetail(displayName, name == "", own, def, name == activeProfile)

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(os.Stdout, "profile", detail)
	}

	printDetail(detail, name == "")

	return nil
}

// profileListed reports whether profiles contains a section with the given
// normalized name. The root profile is never listed; its name is "".
func profileListed(profiles []config.ProfileInfo, name string) bool {
	for _, p := range profiles {
		if p.Name == name {
			return true
		}
	}

	return false
}

// resolveOwnSection returns the named profile's own section, or def when
// name is "" (the default profile).
func resolveOwnSection(name string, def config.ProfileInfo, profiles []config.ProfileInfo) (config.ProfileInfo, error) {
	if name == "" {
		return def, nil
	}

	for _, p := range profiles {
		if p.Name == name {
			return p, nil
		}
	}

	return config.ProfileInfo{}, &config.UnknownProfileError{Name: name, Known: profileNames(profiles)}
}

func profileNames(profiles []config.ProfileInfo) []string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}

	return names
}

// resolveDetail merges own (the named profile's own settings) with def (the
// root profile's settings) so callers can see both the effective value and
// whether it came from the profile itself or was inherited. The root profile
// (isRoot) is its own fallback, so nothing is ever "inherited" for it. The
// explicit profiles.default section is an ordinary profile here: it inherits
// from root exactly like any other named profile.
func resolveDetail(name string, isRoot bool, own, def config.ProfileInfo, active bool) profileDetail {
	detail := profileDetail{Name: name, Active: active}

	// config.applyProfile merges endpoint and token atomically, so a profile
	// that defines either one owns both and never inherits the other.
	credOwn := isRoot || own.Endpoint != "" || own.HasToken

	detail.EndpointOwn = credOwn
	detail.HasTokenOwn = credOwn
	detail.Endpoint = own.Endpoint
	detail.HasToken = own.HasToken

	if !credOwn {
		detail.Endpoint = def.Endpoint
		detail.HasToken = def.HasToken
	}

	detail.CACertOwn = isRoot || own.CACert != ""
	detail.CACert = own.CACert

	if !detail.CACertOwn {
		detail.CACert = def.CACert
	}

	return detail
}

// printDetail renders the human show output. isRoot distinguishes the
// root-level profile (displayed as "default") from an explicit
// profiles.default section, which gets an "(explicit)" suffix so the two
// identities cannot be confused.
func printDetail(d profileDetail, isRoot bool) {
	title := d.Name
	if !isRoot && d.Name == defaultProfileLabel {
		title += " (explicit)"
	}

	if d.Active {
		title += " (active)"
	}

	fmt.Println(tui.SubTitleStyle.Render(title))

	printField("endpoint", valueOrDash(d.Endpoint), d.EndpointOwn)
	printField("token", tokenStatus(d.HasToken), d.HasTokenOwn)
	printField("ca-cert", valueOrDash(d.CACert), d.CACertOwn)
}

func printField(label, value string, own bool) {
	source := ""
	if !own {
		source = tui.DimStyle.Render(" (inherited from default)")
	}

	fmt.Printf("  %s: %s%s\n", tui.InfoStyle.Render(label), value, source)
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

func tokenStatus(hasToken bool) string {
	if hasToken {
		return "set"
	}

	return "not set"
}
