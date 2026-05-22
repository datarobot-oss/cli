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

package log_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config/viperx"
	drlog "github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const debugLogFile = ".dr-tui-debug.log"

// startLogger redirects HOME to a temp dir, calls Start(), and registers
// Stop + viperx.Reset in cleanup so global state is restored after each test.
func startLogger(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	testutil.SetTestHomeDir(t, tmpDir)
	t.Cleanup(func() {
		drlog.Stop()
		viperx.Reset()
	})

	drlog.Start()

	return tmpDir
}

// TestStart verifies that Start() picks up the right log level and verbosity
// for the debug, verbose, and default configurations, and that the log file
// is always created.
func TestStart(t *testing.T) {
	cases := []struct {
		name        string
		debug       bool
		verbose     bool
		wantLevel   log.Level
		wantVerbose bool
	}{
		{"debug", true, false, log.DebugLevel, true},
		{"verbose", false, true, log.InfoLevel, true},
		{"default", false, false, log.InfoLevel, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viperx.Set("debug", tc.debug)
			viperx.Set("verbose", tc.verbose)

			tmpDir := startLogger(t)

			assert.Equal(t, tc.wantLevel, drlog.GetLevel())
			assert.Equal(t, tc.wantVerbose, drlog.IsVerbose())
			assert.FileExists(t, filepath.Join(tmpDir, debugLogFile))
		})
	}
}

// TestLog_WritesToFile exercises every public logging function and confirms
// their output reaches the log file. It also covers the stderrLogger == nil
// branch in Log/Logf by calling StopStderr mid-test.
func TestLog_WritesToFile(t *testing.T) {
	viperx.Set("debug", true)

	tmpDir := startLogger(t)

	drlog.Debug("debug msg")
	drlog.Debugf("debugf %s", "val")
	drlog.Info("info msg")
	drlog.Infof("infof %s", "val")
	drlog.Warn("warn msg")
	drlog.Warnf("warnf %s", "val")
	drlog.Error("error msg")
	drlog.Errorf("errorf %s", "val")
	drlog.Print("print msg")
	drlog.Printf("printf %s", "val")
	drlog.Log(log.DebugLevel, "log msg", "k", "v")
	drlog.Logf(log.DebugLevel, "logf %s", "val")

	content, err := os.ReadFile(filepath.Join(tmpDir, debugLogFile))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug msg")
	assert.Contains(t, string(content), "info msg")
	assert.Contains(t, string(content), "warn msg")
	assert.Contains(t, string(content), "error msg")
	assert.Contains(t, string(content), "logf val")

	// Cover the stderrLogger == nil branch in Log and Logf.
	drlog.StopStderr()
	drlog.Debug("after stderr stopped")
	drlog.Logf(log.InfoLevel, "logf after stop")
}
