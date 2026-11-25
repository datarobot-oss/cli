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
	"crypto/x509"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExportWindowsCerts_IgnoresInheritedModulePath exercises the real Windows
// provider without changing trust stores or contacting an API. The parent path
// deliberately contains no modules, reproducing CFX-6924 before the fix.
func TestExportWindowsCerts_IgnoresInheritedModulePath(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the Windows PowerShell certificate provider")
	}

	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is not installed")
	}

	t.Setenv(psModulePathKey, t.TempDir())

	dest := filepath.Join(t.TempDir(), "ca-bundle.pem")
	err := ExportWindowsCerts(dest)

	if errors.Is(err, ErrNoWindowsCerts) {
		t.Skip("this machine has no certificates to export")
	}

	require.NoError(t, err, "a broken inherited module path must not prevent export")

	bundle, err := os.ReadFile(dest)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(bundle), "the exported bundle must contain a usable certificate")
}
