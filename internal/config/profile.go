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

package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// profilesKey is the top-level drconfig.yaml key holding the named-profile
// sections.
const profilesKey = "profiles"

// DefaultProfileLabel is how the default (top-level, unnamed) profile is
// addressed on the command line and in `dr auth profile` output. The root
// profile's internal name stays "" (see ProfileInfo), but "default" is also
// a legal explicit profile name: a hand-edited or CLI-written
// profiles.default section is selected only by --profile default or
// DATAROBOT_CLI_PROFILE=default, never by omission.
const DefaultProfileLabel = "default"

// ProfileScopedKeys are the persistable keys a named profile may override.
// Every other persistable key (e.g. default-llm-id) is global: it lives only
// at the top level and is shared by every profile.
//
// ssl_verify is deliberately absent: nothing in the CLI reads it (TLS is
// driven by --ca-cert and --skip-certificate-check), so it stays a global
// passthrough value the config writer preserves rather than a profile knob
// that would imply it gates this CLI's TLS behaviour.
var ProfileScopedKeys = map[string]struct{}{
	DataRobotURL:    {},
	DataRobotAPIKey: {},
	"ca-cert":       {},
}

// profileNamePattern is what viper can address as a dotted-path key segment:
// "." is viper's key delimiter, so a profile name cannot contain one.
var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// UnknownProfileError is returned when --profile (or DATAROBOT_CLI_PROFILE)
// names a section that does not exist in the config file, or when `auth
// profile show <name>` is given a name that is not defined. Known holds the
// explicitly defined profile names, sorted; it never contains an entry for
// the root (top-level) credentials.
type UnknownProfileError struct {
	Name  string
	Known []string
}

// Error renders the actionable message shared by the flag/env selection path
// and the positional `auth profile show <name>` path: the normalized missing
// name, the explicit candidate set (or a statement that none are defined),
// and list/create remediation, with no wrapper noise. The normalized name
// "default" gets extra guidance because selecting it looks like the omitted
// selection: omission uses the root-level credentials and never activates
// profiles.default, so the user must create the explicit section or drop the
// selection.
func (e *UnknownProfileError) Error() string {
	name := NormalizeProfileName(e.Name)

	var summary string

	if len(e.Known) == 0 {
		summary = fmt.Sprintf("unknown profile %q; no explicit profiles are defined in drconfig.yaml", name)
	} else {
		summary = fmt.Sprintf("unknown profile %q; defined profiles: %s", name, strings.Join(e.Known, ", "))
	}

	if name == DefaultProfileLabel {
		return strings.Join([]string{
			summary,
			"",
			`Omitting the profile selection uses the root-level credentials, which are`,
			`separate from a profile named "default": omission never activates`,
			`profiles.default, and this config does not define one.`,
			"",
			`To create the explicit "default" profile, run one of:`,
			"  dr auth login --profile default",
			"  dr auth set-url --profile default",
			"Or drop the selection (remove --profile default, or unset",
			"DATAROBOT_CLI_PROFILE) to use the root-level credentials.",
		}, "\n")
	}

	return strings.Join([]string{
		summary,
		"",
		"To list the defined profiles, run:",
		"  dr auth profile list",
		"",
		"To create this profile, run one of:",
		"  dr auth login --profile " + name,
		"  dr auth set-url --profile " + name,
	}, "\n")
}

// NormalizeProfileName lowercases and trims a profile name to match viper's
// case-insensitive config keys.
func NormalizeProfileName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidateProfileName rejects names viper cannot address as a config
// section: empty, or containing characters outside [A-Za-z0-9_-]. The
// normalized name "default" is legal: an explicit profiles.default section
// may be selected and written like any other profile. (Deleting it stays
// reserved; see the auth profile delete command.)
func ValidateProfileName(name string) error {
	if name == "" {
		return errors.New("profile name must not be empty")
	}

	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("profile name %q must start with a letter or digit and contain only letters, digits, '-', or '_'", name)
	}

	return nil
}

// ActiveProfile returns the normalized name of the active profile, or "" for
// the default (top-level) profile.
func ActiveProfile() string {
	return NormalizeProfileName(viper.GetString(ProfileKey))
}

// ProfileDescription names, in user-facing wording, the credentials a given
// normalized profile name resolves to: the root-level credentials for the
// empty name (an omitted selection), otherwise the quoted profile name. It
// is the shared wording behind SelectedCredentialsDescription and the save
// notice emitted by successful auth login/set-url writes.
func ProfileDescription(name string) string {
	if name == "" {
		return "the root-level credentials"
	}

	return fmt.Sprintf("profile %q", name)
}

// SelectedCredentialsDescription names, in user-facing wording, whose
// credentials the active selection resolves to: the root-level credentials
// when selection is omitted, otherwise the explicitly selected profile. The
// name is the normalized one (see ActiveProfile), so status and recovery
// messages match what a follow-up command must spell.
func SelectedCredentialsDescription() string {
	return ProfileDescription(ActiveProfile())
}

// SelectedProfileFlagSuffix returns " --profile <name>" when a profile is
// explicitly selected (flag or environment variable), or "" when selection is
// omitted, so suggested recovery commands retrace the selection that produced
// the problem instead of silently targeting the root credentials.
func SelectedProfileFlagSuffix() string {
	if name := ActiveProfile(); name != "" {
		return " --profile " + name
	}

	return ""
}

// profileSection returns the raw sub-map for a profile as loaded from the
// config file, and whether it exists. name must already be normalized.
func profileSection(name string) (map[string]any, bool) {
	profiles := viper.GetStringMap(profilesKey)

	section, ok := profiles[name]
	if !ok {
		return nil, false
	}

	return toStringKeyedMap(section)
}

// ProfileNames returns the sorted profile names present in the config file.
func ProfileNames() []string {
	return namedProfileKeys(viper.GetStringMap(profilesKey))
}

// namedProfileKeys returns the sorted, addressable section names in a raw
// profiles map. An explicit "profiles.default" section is enumerated like
// any other: it is a legal profile selected by --profile default, and error
// guidance and completion must list it whenever it is defined. The root
// profile is not a section here and is never part of the result.
func namedProfileKeys(profiles map[string]any) []string {
	names := make([]string, 0, len(profiles))

	for name := range profiles {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// applyProfile merges the named profile's scoped keys into viper's config
// layer, so they shadow the top-level keys while still losing to explicit
// flags and environment variables (both of which sit above the config layer
// in viper's own precedence). name must already be normalized.
//
// endpoint and token are merged atomically: if the profile section defines
// either one, both are merged (substituting "" for the one it omits), so a
// profile can never end up pairing its own endpoint with the root profile's
// token or vice versa. This includes an explicit profiles.default section.
func applyProfile(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	section, ok := profileSection(name)
	if !ok {
		return &UnknownProfileError{Name: name, Known: ProfileNames()}
	}

	overlay := map[string]any{}

	_, hasEndpoint := section[DataRobotURL]
	_, hasToken := section[DataRobotAPIKey]

	if hasEndpoint || hasToken {
		overlay[DataRobotURL] = stringOrEmpty(section[DataRobotURL])
		overlay[DataRobotAPIKey] = stringOrEmpty(section[DataRobotAPIKey])
	}

	for key := range ProfileScopedKeys {
		if key == DataRobotURL || key == DataRobotAPIKey {
			continue
		}

		if value, ok := section[key]; ok {
			overlay[key] = value
		}
	}

	if len(overlay) == 0 {
		return nil
	}

	return viper.MergeConfigMap(overlay)
}

// stringOrEmpty returns v as a string, or "" if v is nil or not a string.
func stringOrEmpty(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}

	return s
}

// ProfileInfo is a read-only snapshot of one profile's own settings, exactly
// as stored in drconfig.yaml. Name is "" for the root (top-level) profile;
// an explicit profiles.default section appears under the name "default" and
// is distinct from the root profile. A field is the zero value when the
// profile does not define it (it inherits from the root profile at merge
// time; see applyProfile).
type ProfileInfo struct {
	Name     string
	Endpoint string
	HasToken bool
	CACert   string
}

// LoadProfiles re-reads the config file drconfig.yaml is currently pointed
// at into an isolated viper instance and returns the default profile's own
// settings plus every named profile's own settings, sorted by name.
//
// This is deliberately independent of the process-global viper instance:
// once a named profile is active, applyProfile has overwritten that
// instance's top-level endpoint/token with the profile's values, so it can
// no longer answer "what does the default profile's own endpoint say" --
// exactly the question `dr auth profile list`/`show` need to answer for a
// profile other than the active one.
func LoadProfiles() (ProfileInfo, []ProfileInfo, error) {
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		return ProfileInfo{}, nil, nil
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetConfigType("yaml")

	// SetConfigFile bypasses viper's search, so a missing file surfaces as a
	// plain os.ErrNotExist rather than viper.ConfigFileNotFoundError. Treat it
	// as "no profiles configured" so a config deleted mid-session still lists.
	err := v.ReadInConfig()
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return ProfileInfo{}, nil, nil
	}

	if err != nil {
		return ProfileInfo{}, nil, err
	}

	def := profileInfoFromSection("", v.AllSettings())

	// GetStringMap, not AllSettings()[profilesKey]: AllSettings silently
	// drops a profile section that defines nothing of its own (an empty
	// YAML mapping), which would make a bare `profiles.<name>: {}` section
	// vanish from `dr auth profile list` while still resolving correctly
	// via applyProfile (which uses the same GetStringMap access pattern).
	rawProfiles := v.GetStringMap(profilesKey)

	names := namedProfileKeys(rawProfiles)

	profiles := make([]ProfileInfo, 0, len(names))

	for _, name := range names {
		section, _ := toStringKeyedMap(rawProfiles[name])
		profiles = append(profiles, profileInfoFromSection(name, section))
	}

	return def, profiles, nil
}

// profileInfoFromSection builds a ProfileInfo from a profile's own raw
// section (or, for the default profile, the config file's top-level map).
func profileInfoFromSection(name string, section map[string]any) ProfileInfo {
	return ProfileInfo{
		Name:     name,
		Endpoint: stringOrEmpty(section[DataRobotURL]),
		HasToken: stringOrEmpty(section[DataRobotAPIKey]) != "",
		CACert:   stringOrEmpty(section["ca-cert"]),
	}
}
