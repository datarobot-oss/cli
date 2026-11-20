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

//go:build windows

package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/outputformat"
	internaltls "github.com/datarobot/cli/internal/tls"
	"github.com/spf13/cobra"
)

func registerExportWindowsCertsFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("export-windows-certs", false,
		"export Windows certificate store to the DataRobot CA bundle")
}

// windowsCACertPath returns the path where the exported CA bundle is
// written, reusing the same config directory as drconfig.yaml
// (config.GetConfigDir()) instead of a separate %APPDATA%-derived path.
func windowsCACertPath() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("determining CA bundle path: %w", err)
	}

	return filepath.Join(dir, "ca-bundle.pem"), nil
}

func applyWindowsCerts(cmd *cobra.Command, caCert *string) error {
	exportCerts, _ := cmd.Flags().GetBool("export-windows-certs")
	if !exportCerts {
		return nil
	}

	dest, err := windowsCACertPath()
	if err != nil {
		return fmt.Errorf("--export-windows-certs: %w", err)
	}

	if err := internaltls.ExportWindowsCerts(dest); err != nil {
		// Keep actionable text guidance below its error. JSON failures return to
		// executeRoot instead, preserving the structured stderr envelope.
		if errors.Is(err, internaltls.ErrNoWindowsCerts) &&
			outputformat.GetFormat(cmd) != outputformat.OutputFormatJSON {
			w := cmd.ErrOrStderr()

			ReportError(w, fmt.Errorf("--export-windows-certs: %w", err))
			fprintNoWindowsCertsHelp(w)

			return cli.ErrSilent
		}

		return fmt.Errorf("--export-windows-certs: %w", err)
	}

	*caCert = dest

	return nil
}
