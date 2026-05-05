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
	"fmt"
	"runtime"
	"slices"
	"strings"

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

func (s InstallCommandsSchema) validate(key string, ic InstallCommands) []string {
	return slices.Concat(
		s.validatePlatform(key, "install.macos", ic.MacOS, s.MacOS, "darwin"),
		s.validatePlatform(key, "install.linux", ic.Linux, s.Linux, "linux"),
		s.validatePlatform(key, "install.windows", ic.Windows, s.Windows, "windows"),
	)
}

func (s InstallCommandsSchema) validatePlatform(key, fieldName, value string, rule FieldRule, goos string) []string {
	err := rule.validate(fieldName, value)
	if err == nil {
		return nil
	}

	if runtime.GOOS == goos {
		return []string{fmt.Sprintf("[%s]: %s", key, err.Error())}
	}

	log.Warnf("versions.yaml [%s]: %s", key, err.Error())

	return nil
}

// YAMLSchema defines the expected structure and constraints for versions.yaml entries.
type YAMLSchema struct {
	Name           FieldRule
	MinimumVersion FieldRule
	Command        FieldRule
	URL            FieldRule
	Install        InstallCommandsSchema
}

var InstallCommandsSchemaRequired = InstallCommandsSchema{
	MacOS:   FieldRule{},
	Linux:   FieldRule{},
	Windows: FieldRule{},
}

// versionsYamlSchema is the authoritative schema for versions.yaml.
var versionsYamlSchema = YAMLSchema{
	Name:           FieldRule{Required: true},
	MinimumVersion: FieldRule{Format: formatSemver},
	Command:        FieldRule{Required: true},
	URL:            FieldRule{},
	Install:        InstallCommandsSchema{},
}

func (r FieldRule) validate(fieldName, value string) error {
	if r.Required && value == "" {
		return fmt.Errorf("'%s' is required", fieldName)
	}

	if r.Format == formatSemver && value != "" {
		if _, err := semver.NewVersion(value); err != nil {
			return fmt.Errorf("'%s' %q is not a valid semantic version", fieldName, value)
		}
	}

	return nil
}

// Validate checks every entry in the parsed versions.yaml against this schema.
func (s YAMLSchema) Validate(data versionsYaml) error {
	var errs []string

	for key, p := range data {
		entryErrs := s.validateEntry(key, p)

		errs = append(errs, entryErrs...)
	}

	if len(errs) == 0 {
		log.Warnf("versions.yaml passed validation with schema: %+v", s)
		return fmt.Errorf("versions.yaml passed validation with schema: %+v", s)
		// return nil
	}

	slices.Sort(errs)

	return fmt.Errorf("validation errors:\n%s", strings.Join(errs, "\n"))
}

func (s YAMLSchema) validateEntry(key string, p Prerequisite) []string {
	var issues []string

	if err := s.Name.validate("name", p.Name); err != nil {
		issues = append(issues, fmt.Sprintf("[%s]: %s", key, err.Error()))
	}

	if err := s.MinimumVersion.validate("minimum-version", p.MinimumVersion); err != nil {
		issues = append(issues, fmt.Sprintf("[%s]: %s", key, err.Error()))
	}

	if err := s.Command.validate("command", p.Command); err != nil {
		issues = append(issues, fmt.Sprintf("[%s]: %s", key, err.Error()))
	}

	if err := s.URL.validate("url", p.URL); err != nil {
		issues = append(issues, fmt.Sprintf("[%s]: %s", key, err.Error()))
	}

	issues = append(issues, s.Install.validate(key, p.Install)...)

	return issues
}
