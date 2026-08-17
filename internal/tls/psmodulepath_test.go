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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWindowsPowerShellModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		systemRoot string
		want       string
	}{
		{
			name:       "typical SystemRoot",
			systemRoot: `C:\WINDOWS`,
			want:       `C:\WINDOWS\System32\WindowsPowerShell\v1.0\Modules`,
		},
		{
			name:       "empty SystemRoot falls back to C:\\Windows",
			systemRoot: "",
			want:       `C:\Windows\System32\WindowsPowerShell\v1.0\Modules`,
		},
		{
			name:       "trailing separator is not doubled",
			systemRoot: `D:\Windows\`,
			want:       `D:\Windows\System32\WindowsPowerShell\v1.0\Modules`,
		},
		{
			name:       "relocated system root",
			systemRoot: `E:\CustomWin`,
			want:       `E:\CustomWin\System32\WindowsPowerShell\v1.0\Modules`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, windowsPowerShellModulePath(tt.systemRoot))
		})
	}
}

func TestWithPSModulePath(t *testing.T) {
	t.Parallel()

	const pinned = `C:\WINDOWS\System32\WindowsPowerShell\v1.0\Modules`

	tests := []struct {
		name string
		env  []string
		want []string
	}{
		{
			name: "replaces an inherited pwsh 7 value",
			env: []string{
				"PATH=C:\\bin",
				`PSModulePath=C:\Program Files\PowerShell\7\Modules`,
				"SystemRoot=C:\\WINDOWS",
			},
			want: []string{"PATH=C:\\bin", "SystemRoot=C:\\WINDOWS", "PSModulePath=" + pinned},
		},
		{
			name: "matches the key case-insensitively",
			env:  []string{"psmodulepath=junk", "PATH=C:\\bin"},
			want: []string{"PATH=C:\\bin", "PSModulePath=" + pinned},
		},
		{
			name: "adds the entry when absent",
			env:  []string{"PATH=C:\\bin"},
			want: []string{"PATH=C:\\bin", "PSModulePath=" + pinned},
		},
		{
			name: "drops every duplicate, not just the first",
			env:  []string{"PSModulePath=a", "PATH=C:\\bin", "PSMODULEPATH=b"},
			want: []string{"PATH=C:\\bin", "PSModulePath=" + pinned},
		},
		{
			name: "empty environment still yields the pinned entry",
			env:  nil,
			want: []string{"PSModulePath=" + pinned},
		},
		{
			name: "entries without a separator are preserved",
			env:  []string{"MALFORMED", "PSModulePath=x"},
			want: []string{"MALFORMED", "PSModulePath=" + pinned},
		},
		{
			name: "a key merely prefixed with PSModulePath is left alone",
			env:  []string{"PSModulePathExtra=keep", "PSModulePath=drop"},
			want: []string{"PSModulePathExtra=keep", "PSModulePath=" + pinned},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, withPSModulePath(tt.env, pinned))
		})
	}
}

// TestWithPSModulePathDoesNotMutateInput guards against aliasing: os.Environ()'s
// slice must not be rewritten underneath the caller.
func TestWithPSModulePathDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	env := []string{"PATH=C:\\bin", "PSModulePath=original"}
	before := make([]string, len(env))
	copy(before, env)

	withPSModulePath(env, "pinned")

	assert.Equal(t, before, env)
}
