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
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/viper"
)

// configEnvVar is the environment variable the config initializer resolves
// the config file from when no --config flag value was parsed. It is spelled
// out here because viper derives it from the DATAROBOT_CLI_ prefix plus the
// "config" key rather than from a named constant.
const configEnvVar = "DATAROBOT_CLI_CONFIG"

// CompletionConfigPath resolves which drconfig.yaml a shell-completion
// invocation reads profile names from: the explicitly parsed --config value
// when given, otherwise DATAROBOT_CLI_CONFIG, otherwise the default config
// location. This mirrors the config initializer's resolution order without
// reading or initializing the global config layer, so completion can select
// the right file before (and independently of) the normal pre-run.
func CompletionConfigPath(explicitConfig string) string {
	if explicitConfig != "" {
		return explicitConfig
	}

	if env := os.Getenv(configEnvVar); env != "" {
		return env
	}

	dir, err := GetConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, configFileName)
}

// ProfileNamesFromFile returns the explicit profile names defined in the
// config file at path: normalized like --profile selection (lowercase,
// trimmed), de-duplicated (viper's config keys are case-insensitive), and
// sorted. The root-level credentials are not a section and are never part of
// the result; "default" appears exactly when profiles.default is defined.
//
// The read uses an isolated viper instance — like LoadProfiles — so it never
// touches the global config layer, mutates nothing, and makes no network
// request, which is what lets completion run before the normal pre-run
// without side effects. A missing file defines no profiles (nil, nil); an
// unparsable file is an error so the caller can stay silent rather than offer
// wrong candidates.
func ProfileNamesFromFile(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	err := v.ReadInConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return normalizedProfileNames(namedProfileKeys(v.GetStringMap(profilesKey))), nil
}

// normalizedProfileNames normalizes raw section names for completion and
// drops duplicates that normalize together, keeping the result sorted.
func normalizedProfileNames(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))

	names := make([]string, 0, len(raw))

	for _, rawName := range raw {
		name := NormalizeProfileName(rawName)

		if name == "" {
			continue
		}

		if _, dup := seen[name]; dup {
			continue
		}

		seen[name] = struct{}{}

		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
