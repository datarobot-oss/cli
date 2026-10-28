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
// settings for --output-format json. ca_cert is always emitted (the empty
// string when unset) so consumers can rely on the key being present; the
// remaining fields keep their original meaning.
type profileDetail struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Endpoint    string `json:"endpoint"`
	EndpointOwn bool   `json:"endpoint_own"`
	HasToken    bool   `json:"has_token"`
	HasTokenOwn bool   `json:"has_token_own"`
	CACert      string `json:"ca_cert"`
	CACertOwn   bool   `json:"ca_cert_own"`
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "🔍 Show a profile's resolved settings",
		Long: `Show a profile's resolved settings: its own endpoint/token/ca-cert,
falling back to the root-level credentials' values for anything it doesn't
define.

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

	// Human output names the config file the settings were read from, so the
	// user can tell which drconfig.yaml an inspection came from. JSON output
	// must stay a single pure document, so the path is human-only.
	configPath := ""
	if path, err := config.ConfigFilePath(); err == nil {
		configPath = path
	}

	printDetail(detail, name == "", configPath, own)

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
	// that defines either one owns both and never inherits the other; the
	// resolved values below follow that pairing. The *_own flags report
	// ownership of that atomic pair, not per-key presence: both are true
	// whenever the profile defines an endpoint, a token, or both, and both
	// are false when it defines neither (empty or CA-only), in which case
	// the root pair is inherited whole. Only the resolved Endpoint/HasToken
	// fields distinguish which individual values are present. The root
	// profile is its own fallback and always reports ownership (an existing
	// contract, kept for ca_cert_own below as well).
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
// profiles.default section, configPath names the drconfig.yaml the settings
// were read from (empty when it could not be resolved), and own is the
// inspected section's own settings, which drive the unpairing explanation.
func printDetail(d profileDetail, isRoot bool, configPath string, own config.ProfileInfo) {
	title := d.Name

	if isRoot {
		title += " (root-level)"
	} else if d.Name == defaultProfileLabel {
		title += " (explicit)"
	}

	if d.Active {
		title += " (active)"
	}

	fmt.Println(tui.SubTitleStyle.Render(title))

	if configPath != "" {
		fmt.Printf("  %s: %s\n", tui.InfoStyle.Render("config file"), configPath)
	}

	// The credential pair resolves as a unit: whenever the profile owns
	// either key it owns both resolutions, so a missing half is absent, not
	// inherited. Only a profile that owns neither borrows from root.
	pairOwn := isRoot || d.EndpointOwn || d.HasTokenOwn

	printField("endpoint", valueOrDash(d.Endpoint), !isRoot && !pairOwn)
	printField("token", tokenStatus(d.HasToken), !isRoot && !pairOwn)
	printField("ca-cert", valueOrDash(d.CACert), !d.CACertOwn)

	printUnpairingExplanation(isRoot, own.Endpoint != "", own.HasToken)
}

// printUnpairingExplanation renders the human note explaining why a profile
// that owns only one credential never borrows the other from the root-level
// credentials: endpoint and token are saved and resolved as an atomic pair,
// so a half-defined profile leaves the other half empty rather than mixing
// sources. The explanation is derived from the section's own endpoint/token
// values rather than the JSON *_own flags, which report pair-wide ownership
// and can no longer distinguish an endpoint-only from a token-only profile.
// A profile owning both or neither needs no note, and the root profile is
// its own fallback.
func printUnpairingExplanation(isRoot, ownEndpoint, ownToken bool) {
	if isRoot || ownEndpoint == ownToken {
		return
	}

	if ownEndpoint {
		fmt.Printf("  %s\n", tui.DimStyle.Render(
			"This profile has its own endpoint and no token of its own; it does not use the root-level token (endpoint and token are always paired)."))
	} else {
		fmt.Printf("  %s\n", tui.DimStyle.Render(
			"This profile has its own token and no endpoint of its own; it does not inherit the root-level endpoint (endpoint and token are always paired)."))
	}
}

// printField renders one "label: value" line. inherited dims an explicit
// root-source suffix onto values that were resolved from the root-level
// credentials rather than the inspected profile; the wording names the root
// source unambiguously because the inspected profile may itself be called
// "default" (the explicit profiles.default section).
func printField(label, value string, inherited bool) {
	source := ""
	if inherited {
		source = tui.DimStyle.Render(" (inherited from the root-level credentials)")
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
