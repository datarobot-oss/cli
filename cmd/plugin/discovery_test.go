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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanTLSArgs(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		expectedSkipVerify bool
		expectedCACert     string
	}{
		{
			name:               "no flags",
			args:               []string{"foo", "bar"},
			expectedSkipVerify: false,
			expectedCACert:     "",
		},
		{
			name:               "short skip-verify flag",
			args:               []string{"-k", "foo"},
			expectedSkipVerify: true,
			expectedCACert:     "",
		},
		{
			name:               "long skip-verify flag",
			args:               []string{"--skip-certificate-check", "foo"},
			expectedSkipVerify: true,
			expectedCACert:     "",
		},
		{
			name:               "ca-cert space-separated form",
			args:               []string{"--ca-cert", "/path/to/ca.pem"},
			expectedSkipVerify: false,
			expectedCACert:     "/path/to/ca.pem",
		},
		{
			name:               "ca-cert equals form",
			args:               []string{"--ca-cert=/path/to/ca.pem"},
			expectedSkipVerify: false,
			expectedCACert:     "/path/to/ca.pem",
		},
		{
			name:               "ca-cert with dash-leading value",
			args:               []string{"--ca-cert", "-my-cert.pem"},
			expectedSkipVerify: false,
			expectedCACert:     "-my-cert.pem",
		},
		{
			name:               "unknown flags pass through without error",
			args:               []string{"--some-plugin-flag", "value", "-k"},
			expectedSkipVerify: true,
			expectedCACert:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipVerify, caCert := scanTLSArgs(tt.args)

			assert.Equal(t, tt.expectedSkipVerify, skipVerify)
			assert.Equal(t, tt.expectedCACert, caCert)
		})
	}
}
