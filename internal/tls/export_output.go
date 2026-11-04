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
	"fmt"
	"strconv"
	"strings"
)

// certCountPrefix marks the first stdout line of the export script, which reports how
// many certificates were enumerated before any encoding was attempted.
const certCountPrefix = "CERTCOUNT="

// No build constraint: only the Windows build calls these, but the parsing is where a
// regression would quietly bring back the misleading "no certificates found" message,
// so it is unit tested on every host.

// parseExportOutput splits the export script's stdout into its CERTCOUNT header and
// the PEM body. The header is what lets the caller tell an empty store apart from a
// store it could not encode.
func parseExportOutput(raw string) (pem string, count int, err error) {
	trimmed := strings.TrimSpace(raw)

	header, body, _ := strings.Cut(trimmed, "\n")
	header = strings.TrimSpace(header)

	if !strings.HasPrefix(header, certCountPrefix) {
		return "", 0, fmt.Errorf(
			"exporting Windows cert store: unexpected output from powershell.exe: %s",
			truncateForError(trimmed),
		)
	}

	count, convErr := strconv.Atoi(strings.TrimPrefix(header, certCountPrefix))
	if convErr != nil {
		return "", 0, fmt.Errorf(
			"exporting Windows cert store: unreadable certificate count %q: %w", header, convErr,
		)
	}

	return strings.TrimSpace(body), count, nil
}

// truncateForError keeps unexpected subprocess output short enough to read in a
// terminal while still showing what came back.
func truncateForError(s string) string {
	const maxLen = 200

	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")

	if len(s) > maxLen {
		return strconv.Quote(s[:maxLen] + "…")
	}

	return strconv.Quote(s)
}
