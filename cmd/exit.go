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

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/datarobot/cli/internal/cli"
)

// ReportError writes a user-facing "Error: ..." line for err to w, unless err is
// cli.ErrSilent — the sentinel meaning a command already printed its own message.
//
// RootCmd sets SilenceErrors so cobra never prints errors itself, which makes this
// the single place errors reach the user. Previously cobra was the only reporter,
// and every command setting SilenceErrors: true silently swallowed failures raised
// by the root PersistentPreRunE (config loading, TLS setup) that it never had a
// chance to print — a bad --ca-cert path exited 1 with no output at all (CFX-6924).
func ReportError(w io.Writer, err error) {
	if err == nil || errors.Is(err, cli.ErrSilent) {
		return
	}

	fmt.Fprintln(w, "Error:", err)
}

// Exit flushes any pending telemetry events then terminates the process with
// code. Call this from main (instead of os.Exit) when ExecuteContext returns
// an error, so that Amplitude events are delivered even though
// PersistentPostRunE is bypassed on the error path.
//
// telemetryClient is set in PersistentPreRunE (root.go); it will be nil only
// if the process exits before any command runs (e.g. flag parse failure), in
// which case there are no queued events to flush anyway.
func Exit(code int) {
	if telemetryClient != nil {
		telemetryClient.Flush(3 * time.Second)
	}

	// This is the only place in the codebase that should call os.Exit
	os.Exit(code) //nolint:forbidigo
}
