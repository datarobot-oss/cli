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

// Package open launches URLs in the user's default browser.
package open

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// ErrUnsupportedURL is returned when a URL is not a plain http(s) URL and so is
// not safe to hand to a platform launcher.
var ErrUnsupportedURL = errors.New("only http and https URLs can be opened in a browser")

// browserCommand returns the executable and arguments that open rawURL in the
// default browser on goos.
//
// The URL is validated first: it is passed to an external launcher, so anything
// that is not a plain http(s) URL (a file:// path, a javascript: payload, a
// leading dash that a launcher might read as a flag) is rejected rather than
// forwarded.
func browserCommand(goos, rawURL string) (string, []string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %q is not a valid URL", ErrUnsupportedURL, rawURL)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("%w: got scheme %q", ErrUnsupportedURL, parsed.Scheme)
	}

	if parsed.Host == "" {
		return "", nil, fmt.Errorf("%w: %q has no host", ErrUnsupportedURL, rawURL)
	}

	// A launcher may treat a leading dash as one of its own flags, and no
	// legitimate URL contains whitespace at this point.
	if strings.HasPrefix(rawURL, "-") || strings.ContainsAny(rawURL, " \t\n\r") {
		return "", nil, fmt.Errorf("%w: %q contains unsupported characters", ErrUnsupportedURL, rawURL)
	}

	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		// "start" is a cmd.exe builtin with no executable on disk, so
		// exec.Command("start", url) can never succeed. rundll32 is a real
		// binary, needs no shell quoting for "&" in query strings, and opens no
		// console window.
		return "rundll32.exe", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "xdg-open", []string{rawURL}, nil
	}
}

// Open launches rawURL in the user's default browser.
//
// It returns an error when the URL is unsupported or the platform launcher is
// missing or exits non-zero, so callers can fall back to showing the user the
// link instead of waiting on a browser that never opened.
func Open(rawURL string) error {
	if testing.Testing() {
		return nil
	}

	name, args, err := browserCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}

	// The executable name is a constant per platform and rawURL is validated by
	// browserCommand above, so no untrusted input reaches the command line.
	cmd := exec.Command(name, args...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open browser via %s: %w", name, err)
	}

	return nil
}
