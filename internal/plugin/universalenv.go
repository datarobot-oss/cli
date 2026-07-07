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
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/pflag"
)

// UniversalAnnotationKey is the pflag annotation key that cmd/root.go sets on
// any persistent flag that should be forwarded to plugin subprocesses as a
// DATAROBOT_CLI_<suffix> env var. The annotation value is a single-element
// slice holding the env var suffix (e.g. ["DEBUG"] → DATAROBOT_CLI_DEBUG).
// internal/plugin only reads this annotation — it never writes it.
const UniversalAnnotationKey = "plugin-universal"

// rootFlags holds the root command's persistent flag set, registered via
// SetRootFlags during CLI initialisation.
var rootFlags *pflag.FlagSet

// SetRootFlags registers the root command's persistent flags so that
// universalFlagEnv can discover which flags are annotated as universal.
// Called once from RegisterPluginCommands in cmd/plugin/discovery.go.
func SetRootFlags(fs *pflag.FlagSet) {
	rootFlags = fs
}

// universalFlagEnv returns "KEY=VALUE" strings for every persistent root flag
// carrying a UniversalAnnotationKey annotation. Values are read from viper
// (the authoritative source after flag parsing).
func universalFlagEnv() []string {
	if rootFlags == nil {
		return nil
	}

	var env []string

	rootFlags.VisitAll(func(flag *pflag.Flag) {
		suffixes, ok := flag.Annotations[UniversalAnnotationKey]
		if !ok || len(suffixes) == 0 {
			return
		}

		envKey := config.EnvPrefix + suffixes[0]

		if flag.Value.Type() == "bool" {
			if viperx.GetBool(flag.Name) {
				env = append(env, envKey+"=1")
			}

			return
		}

		if val := viperx.GetString(flag.Name); val != "" {
			env = append(env, envKey+"="+val)
		}
	})

	return env
}
