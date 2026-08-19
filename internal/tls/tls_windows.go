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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoWindowsCerts reports that enumerating the certificate stores succeeded but
// found nothing. It is a sentinel so callers can offer guidance: an empty store is a
// machine-configuration problem the user can act on, unlike a failure to reach the
// store at all, which the script reports separately.
var ErrNoWindowsCerts = errors.New(
	"no certificates found in the Windows certificate store: " +
		"Root and CA are empty for both LocalMachine and CurrentUser",
)

// ExportWindowsCerts exports Root and CA certificates from the Windows
// certificate store and writes them as a PEM bundle to dest.
func ExportWindowsCerts(dest string) error {
	script := strings.Join([]string{
		// Load the module providing the Cert: drive explicitly and fail loudly when it
		// is unavailable. Relying on autoloading turned a broken PSModulePath into a
		// silent empty result that reported itself as "no certificates found".
		`Import-Module Microsoft.PowerShell.Security -ErrorAction Stop`,
		`if (-not (Get-PSDrive -PSProvider Certificate -ErrorAction SilentlyContinue)) ` +
			`{ throw 'Cert: drive unavailable - Microsoft.PowerShell.Security did not load' }`,
		`$out = @()`,
		`foreach ($store in 'Root','CA') {`,
		`  foreach ($loc in 'LocalMachine','CurrentUser') {`,
		// SilentlyContinue is kept here on purpose: an individual store may legitimately
		// be empty or absent. The precondition above distinguishes that from the whole
		// certificate provider being missing.
		`    Get-ChildItem -Path "Cert:\$loc\$store" -ErrorAction SilentlyContinue | ForEach-Object {`,
		`      $out += '-----BEGIN CERTIFICATE-----'`,
		`      $out += [Convert]::ToBase64String($_.RawData, 'InsertLineBreaks')`,
		`      $out += '-----END CERTIFICATE-----'`,
		`    }`,
		`  }`,
		`}`,
		`$out -join [Environment]::NewLine`,
	}, "; ")

	cmd := exec.Command(
		"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script,
	)

	// Pin PSModulePath instead of inheriting it; see withPSModulePath for why.
	cmd.Env = withPSModulePath(
		os.Environ(),
		windowsPowerShellModulePath(os.Getenv("SystemRoot")),
	)

	out, err := cmd.Output()
	if err != nil {
		// cmd.Output captures the child's stderr into ExitError.Stderr. Surfacing it is
		// the difference between "something went wrong" and a message naming the cause.
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				return fmt.Errorf("exporting Windows cert store: %w: %s", err, stderr)
			}
		}

		return fmt.Errorf("exporting Windows cert store: %w", err)
	}

	pem := strings.TrimSpace(string(out))

	if pem == "" {
		return ErrNoWindowsCerts
	}

	if err := os.WriteFile(dest, []byte(pem+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing CA bundle to %q: %w", dest, err)
	}

	return nil
}
