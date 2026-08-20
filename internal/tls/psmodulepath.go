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

package tls

import "strings"

// psModulePathKey is the environment variable telling PowerShell where to look for
// modules. Matched case-insensitively, since Windows environment variable names are.
const psModulePathKey = "PSMODULEPATH"

// defaultSystemRoot is the fallback when SystemRoot is absent from the environment.
const defaultSystemRoot = `C:\Windows`

// These helpers deliberately carry no build constraint so they can be unit tested on
// any host, even though only the Windows build calls them.

// windowsPowerShellModulePath returns the Windows PowerShell 5.1 system module
// directory under systemRoot. That directory holds Microsoft.PowerShell.Security,
// the module providing the Cert: drive — without it on PSModulePath, powershell.exe
// cannot enumerate the certificate stores at all.
//
// The path is assembled with backslashes rather than filepath.Join so the result is
// identical no matter which OS runs the tests.
func windowsPowerShellModulePath(systemRoot string) string {
	if systemRoot == "" {
		systemRoot = defaultSystemRoot
	}

	return strings.Join([]string{
		strings.TrimRight(systemRoot, `\`),
		"System32",
		"WindowsPowerShell",
		"v1.0",
		"Modules",
	}, `\`)
}

// withPSModulePath returns env with every PSModulePath entry replaced by a single
// entry pointing at modulePath.
//
// A child powershell.exe otherwise inherits the parent's PSModulePath, and a value
// omitting the Windows PowerShell system module directory stops
// Microsoft.PowerShell.Security from autoloading. The Cert: drive then does not
// exist, the export yields nothing, and the failure surfaces as the misleading
// "no certificates found in Windows cert store" (CFX-6924). pwsh 7 sets exactly
// such a value, and CI runners frequently mangle it.
func withPSModulePath(env []string, modulePath string) []string {
	out := make([]string, 0, len(env)+1)

	for _, entry := range env {
		key, _, found := strings.Cut(entry, "=")

		if found && strings.EqualFold(key, psModulePathKey) {
			continue
		}

		out = append(out, entry)
	}

	return append(out, "PSModulePath="+modulePath)
}
