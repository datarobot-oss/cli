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

package doctor

import (
	"errors"
	"fmt"
	"path/filepath"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// Stable check identifiers. They surface in reports and JSON output, so they
// must not change between releases.
const (
	CheckIDPresence   = "wapi.presence"
	CheckIDConfig     = "wapi.config"
	CheckIDManifest   = "wapi.manifest"
	CheckIDDivergence = "wapi.config-manifest-divergence"
	CheckIDRollback   = "wapi.rollback"
	CheckIDLock       = "wapi.lock"
)

// Checks returns the complete doctor check suite in pinned order: the six
// local checks then the four remote checks (ten total; future extras append
// after). The remote checks share one artifact snapshot fetched through store.
//
// Each check resolves projectDir independently at Run time, so the returned
// checks stay correct even if the directory's state changes between
// construction and execution.
func Checks(projectDir string, store ArtifactGetter) []core.Check {
	return append(LocalChecks(projectDir), RemoteChecks(projectDir, store)...)
}

// LocalChecks returns the six local sync-state checks in fixed report order:
// presence, config, manifest, divergence, rollback, lock. The remote checks
// append after these.
//
// Each check resolves projectDir independently at Run time, so the returned
// checks stay correct even if the directory's state changes between
// construction and execution.
func LocalChecks(projectDir string) []core.Check {
	return []core.Check{
		&presenceCheck{projectDir: projectDir},
		&configCheck{projectDir: projectDir},
		&manifestCheck{projectDir: projectDir},
		&divergenceCheck{projectDir: projectDir},
		&rollbackCheck{projectDir: projectDir},
		newLockCheck(projectDir),
	}
}

// skipIfUnlinked implements the presence-FAIL cascade: a check that needs
// linked state SKIPs with an honest "no linked state" summary rather than a
// misleading FAIL of its own. The second return value reports whether the
// caller should skip.
func skipIfUnlinked(projectDir string) (core.Result, bool) {
	if wapi.Exists(projectDir) {
		return core.Result{}, false
	}

	return core.Result{
		Status:  core.StatusSKIP,
		Summary: "no linked state; nothing to check",
	}, true
}

// corruptFileResult builds a FAIL result for a missing or unreadable state
// file. The absolute path appears in both the summary and details.path for
// JSON consumers.
func corruptFileResult(summary, path, remedy string, fixable bool) core.Result {
	abs := absPath(path)

	return core.Result{
		Status:  core.StatusFAIL,
		Summary: fmt.Sprintf("%s (%s)", summary, abs),
		Remedy:  remedy,
		Details: map[string]string{"path": abs},
		Fixable: fixable,
	}
}

// stateErrPath extracts the file path carried by a wapi.CorruptedError,
// falling back to fallbackPath when err is not one.
func stateErrPath(err error, fallbackPath string) string {
	var corruptErr *wapi.CorruptedError

	if errors.As(err, &corruptErr) {
		return corruptErr.Path
	}

	return fallbackPath
}

// corruptReason returns the most specific message for a corrupted state
// file: the underlying cause of a wapi.CorruptedError (whose Error text
// already embeds the path, which corruptFileResult appends once), or the
// error itself otherwise.
func corruptReason(err error) string {
	var corruptErr *wapi.CorruptedError

	if errors.As(err, &corruptErr) && corruptErr.Err != nil {
		return corruptErr.Err.Error()
	}

	return err.Error()
}

// absPath converts p to its absolute form. On the (non-representable) error
// path it returns p unchanged: callers already pass an absolute project dir
// in normal wiring (the command resolves --dir with filepath.Abs).
func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}

	return abs
}

// normalizeStringPtr treats an empty string as absent, so a pointer to ""
// compares equal to nil everywhere the doctor reasons about config/manifest
// pointer fields (the "empty ≈ nil" normalization pinned for all checks).
func normalizeStringPtr(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}

	return p
}

// ptrDisplay renders an optional pointer for human/JSON summaries: the value,
// or "null" when absent (after empty-string normalization).
func ptrDisplay(p *string) string {
	if normalizeStringPtr(p) == nil {
		return "null"
	}

	return *p
}
