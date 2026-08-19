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
	"os"
	"time"
)

// Exit flushes any pending telemetry events then terminates the process with
// code. Call this from main (instead of os.Exit) when ExecuteContext returns
// an error, so that Amplitude events are delivered even though
// PersistentPostRunE is bypassed on the error path.
//
// The telemetry client is retrieved from productionFactory (set in
// PersistentPreRunE) at flush time. It will be nil only if the process exits
// before any command runs (e.g. a flag-parse failure), in which case there
// are no queued events to flush.
func Exit(code int) {
	if c := productionFactory.TelemetryClient(); c != nil {
		c.Flush(3 * time.Second)
	}

	// This is the only place in the codebase that should call os.Exit.
	os.Exit(code) //nolint:forbidigo
}
