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

package features

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const AnnotationKey = "feature-gate"

// Provider defines the interface for reading feature gate values.
// Implementations can read from environment variables, config files, or other sources.
// This allows for easy migration to different feature flag systems without changing
// the rest of the codebase.
type Provider interface {
	IsEnabled(name string) bool
}

// envVarProvider reads feature gates from environment variables.
type envVarProvider struct{}

// IsEnabled checks if a feature is enabled via environment variable.
// Feature names are converted to uppercase with underscores replacing hyphens.
// Format: DATAROBOT_CLI_FEATURE_<NAME> (e.g., DATAROBOT_CLI_FEATURE_WORKLOAD)
func (p *envVarProvider) IsEnabled(name string) bool {
	envKey := "DATAROBOT_CLI_FEATURE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if v := os.Getenv(envKey); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}

	return false
}

// provider is the global feature gate provider instance.
// Default is environment variable provider.
// Can be swapped for testing or to support other sources (e.g., config files, remote services).
var provider Provider = &envVarProvider{}

// SetProvider sets the global feature gate provider.
// Primarily useful for testing with custom implementations.
func SetProvider(p Provider) {
	if p != nil {
		provider = p
	}
}

// Enabled checks if a feature is enabled using the current provider.
// Currently uses environment variables; can be extended to support config files
// or other sources by implementing the Provider interface.
// TODO: Support config file (drconfig.yaml) feature gates once
// we move filtering to PersistentPreRunE or read config independently.
func Enabled(name string) bool {
	return provider.IsEnabled(name)
}

// SetGate adds a feature gate annotation to a command, preserving any existing annotations.
func SetGate(cmd *cobra.Command, name string) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}

	cmd.Annotations[AnnotationKey] = name
}
