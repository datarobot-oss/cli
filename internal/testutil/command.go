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

package testutil

import (
	"io"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

// CaptureStdout redirects os.Stdout while fn runs and returns the captured text.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	t.Cleanup(func() { os.Stdout = orig })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stdout = w

	fn()

	os.Stdout = orig

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

// WireCompletionCommandPath attaches a command to the real completion parent path.
func WireCompletionCommandPath(cmd *cobra.Command) {
	root := &cobra.Command{Use: version.CliName}
	selfCmd := &cobra.Command{Use: "self"}
	completionCmd := &cobra.Command{Use: "completion"}

	completionCmd.AddCommand(cmd)
	selfCmd.AddCommand(completionCmd)
	root.AddCommand(selfCmd)
}
