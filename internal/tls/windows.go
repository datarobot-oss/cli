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

package tls

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExportWindowsCerts exports Root and CA certificates from the Windows
// certificate store and writes them as a PEM bundle to dest.
func ExportWindowsCerts(dest string) error {
	script := strings.Join([]string{
		`$out = @()`,
		`foreach ($store in 'Root','CA') {`,
		`  foreach ($loc in 'LocalMachine','CurrentUser') {`,
		`    try {`,
		`      Get-ChildItem -Path "Cert:\$loc\$store" -ErrorAction SilentlyContinue | ForEach-Object {`,
		`        $out += '-----BEGIN CERTIFICATE-----'`,
		`        $out += [Convert]::ToBase64String($_.RawData, 'InsertLineBreaks')`,
		`        $out += '-----END CERTIFICATE-----'`,
		`      }`,
		`    } catch {}`,
		`  }`,
		`}`,
		`$out -join [Environment]::NewLine`,
	}, "; ")

	out, err := exec.Command(
		"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script,
	).Output()
	if err != nil {
		return fmt.Errorf("exporting Windows cert store: %w", err)
	}

	pem := strings.TrimSpace(string(out))

	if pem == "" {
		return fmt.Errorf("no certificates found in Windows cert store")
	}

	if err := os.WriteFile(dest, []byte(pem+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing CA bundle to %q: %w", dest, err)
	}

	return nil
}
