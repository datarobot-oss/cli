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

package cmd

import (
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/features"
	internaltls "github.com/datarobot/cli/internal/tls"
	"github.com/spf13/cobra"
)

// setupTLS configures http.DefaultTransport based on TLS-related flags and
// the persisted ca-cert config value. Must run after initializeConfig so that
// the ca-cert value from drconfig.yaml is available via viper.
//
// Gated behind "private-ca" (DATAROBOT_CLI_FEATURE_PRIVATE_CA) while this
// design is still subject to change; a no-op when the gate is disabled
// (the default), regardless of what the persisted config contains.
func setupTLS(cmd *cobra.Command) error {
	if !features.Enabled("private-ca") {
		return nil
	}

	skipVerify, _ := cmd.Flags().GetBool("skip-certificate-check")
	caCert := viperx.GetString("ca-cert")

	if err := applyWindowsCerts(cmd, &caCert); err != nil {
		return err
	}

	opts := internaltls.Options{
		SkipVerify: skipVerify,
		CACertPath: caCert,
	}

	if err := internaltls.Apply(opts); err != nil {
		return err
	}

	return internaltls.PropagateEnv(opts)
}
