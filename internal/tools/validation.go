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

package tools

import (
	"runtime"

	semver "github.com/Masterminds/semver/v3"

	"github.com/datarobot/cli/internal/log"
)

const formatSemver = "semver"

// FieldRule defines validation constraints for a single string field.
type FieldRule struct {
	Required bool
	Format   string // "semver" validates semantic version format when non-empty
}

// InstallCommandsSchema defines validation constraints for InstallCommands fields.
type InstallCommandsSchema struct {
	MacOS   FieldRule
	Linux   FieldRule
	Windows FieldRule
}

// YAMLSchema defines the expected structure and constraints for versions.yaml entries.
type YAMLSchema struct {
	Name           FieldRule
	MinimumVersion FieldRule
	Command        FieldRule
	URL            FieldRule
	Install        InstallCommandsSchema
}

// installCommandsSchema is the authoritative schema for InstallCommands in versions.yaml.
// As for Milestone 1, we decided to make Windows install command optional .
// We can always add a warning in validation if Windows command is not provided, but it won't be a hard requirement.
var installCommandsSchema = InstallCommandsSchema{
	MacOS: FieldRule{Required: true},
	Linux: FieldRule{Required: true},
	// InstallCommands for Windows is optional for now.
	Windows: FieldRule{Required: false},
}

// versionsYamlSchema is the authoritative schema for versions.yaml.
var versionsYamlSchema = YAMLSchema{
	Name:           FieldRule{Required: true},
	MinimumVersion: FieldRule{Required: true, Format: formatSemver},
	Command:        FieldRule{Required: true},
	URL:            FieldRule{Required: true},
	Install:        installCommandsSchema,
}

// validate logs a warning if value violates the field rule.
func (r FieldRule) validate(key, fieldName, value string) {
	if r.Required && value == "" {
		log.Warnf("versions.yaml [%s]: '%s' is required", key, fieldName)

		return
	}

	if r.Format == formatSemver && value != "" {
		if _, err := semver.NewVersion(value); err != nil {
			log.Warnf("versions.yaml [%s]: '%s' %q is not a valid semantic version", key, fieldName, value)
		}
	}
}

// Validate checks every entry in the parsed versions.yaml and logs warnings for violations.
func (s YAMLSchema) Validate(data versionsYaml) {
	for key, p := range data {
		s.Name.validate(key, "name", p.Name)
		s.MinimumVersion.validate(key, "minimum-version", p.MinimumVersion)
		s.Command.validate(key, "command", p.Command)
		s.URL.validate(key, "url", p.URL)
		s.Install.validate(key, p.Install)
	}
}

// validate checks that install commands are provided
// according to the InstallCommandsSchema rules, with special attention to the current platform.
func (s InstallCommandsSchema) validate(key string, ic InstallCommands) {
	if ic.MacOS == "" && ic.Linux == "" && ic.Windows == "" {
		log.Warnf("versions.yaml [%s]: 'install' is not defined", key)

		return
	}

	s.validatePlatform(key, "install.macos", ic.MacOS, s.MacOS, "darwin")
	s.validatePlatform(key, "install.linux", ic.Linux, s.Linux, "linux")
	s.validatePlatform(key, "install.windows", ic.Windows, s.Windows, "windows")
}

// validatePlatform logs Error if the install command for the current platform is missing and it's required, otherwise logs a warning.
func (s InstallCommandsSchema) validatePlatform(key, fieldName, value string, rule FieldRule, goos string) {
	if !rule.Required || value != "" {
		return
	}

	// For now it's only warning if the install command for the current platform is missing
	if runtime.GOOS == goos {
		log.Errorf("versions.yaml [%s]: '%s' is required for the current platform", key, fieldName)

		return
	}

	log.Warnf("versions.yaml [%s]: '%s' is required", key, fieldName)
}
