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

package open

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserCommand_PerPlatform(t *testing.T) {
	const url = "https://app.datarobot.com/account/developer-tools?cliRedirect=true"

	tests := map[string]struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		"macOS uses open": {
			goos:     "darwin",
			wantName: "open",
			wantArgs: []string{url},
		},
		// "start" is a cmd.exe builtin with no executable on disk, so
		// exec.Command("start", url) always fails. rundll32 is a real binary and
		// needs no shell quoting for "&" in query strings.
		"Windows uses rundll32, not the cmd builtin": {
			goos:     "windows",
			wantName: "rundll32.exe",
			wantArgs: []string{"url.dll,FileProtocolHandler", url},
		},
		"Linux uses xdg-open": {
			goos:     "linux",
			wantName: "xdg-open",
			wantArgs: []string{url},
		},
		"other Unix falls back to xdg-open": {
			goos:     "freebsd",
			wantName: "xdg-open",
			wantArgs: []string{url},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotName, gotArgs, err := browserCommand(tc.goos, url)

			require.NoError(t, err)
			assert.Equal(t, tc.wantName, gotName)
			assert.Equal(t, tc.wantArgs, gotArgs)
		})
	}
}

func TestBrowserCommand_RejectsNonHTTPURLs(t *testing.T) {
	// The URL is handed to a platform launcher, so anything that is not a plain
	// http(s) URL is rejected rather than passed through.
	tests := map[string]string{
		"empty":               "",
		"file scheme":         "file:///etc/passwd",
		"javascript scheme":   "javascript:alert(1)",
		"no scheme":           "app.datarobot.com",
		"shell metacharacter": "https://app.datarobot.com/;rm -rf /",
		"leading dash":        "-oProxyCommand=evil",
		"not a URL":           "://",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := browserCommand("darwin", url)

			require.Error(t, err, "browserCommand(%q) should be rejected", url)
		})
	}
}

func TestBrowserCommand_AllowsHTTPAndHTTPS(t *testing.T) {
	for _, url := range []string{
		"http://localhost:51164/?key=abc",
		"https://app.eu.datarobot.com/account/developer-tools?cliRedirect=true",
		"https://datarobot.example.com:8443/account/developer-tools?cliRedirect=true&x=1",
	} {
		t.Run(url, func(t *testing.T) {
			name, args, err := browserCommand("darwin", url)

			require.NoError(t, err)
			assert.Equal(t, "open", name)
			assert.Equal(t, []string{url}, args)
		})
	}
}

func TestOpen_NoOpUnderTest(t *testing.T) {
	// The testing.Testing() guard keeps the suite from launching real browsers.
	// A malformed URL still returns nil here because the guard short-circuits
	// before validation - that is intentional, validation is covered above.
	assert.NoError(t, Open("https://app.datarobot.com"))
}
