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

package wapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// HistoryEntry is one JSONL record in .wapi/history.log. It is deliberately
// open-schema: each operation type owns its own field shape. Callers SHOULD
// populate at least "ts" (RFC3339 UTC timestamp) and "op".
type HistoryEntry map[string]any

// AppendHistory writes entry as a single JSON object followed by "\n" to
// .wapi/history.log. If the existing log has grown to historyRotateBytes or
// more, it is renamed to history.log.1 first (overwriting any prior .1 —
// only one backup is retained).
//
// Returns ErrNotInitialized if .wapi/ does not exist. Unlike SaveConfig /
// SaveManifest, appends use O_APPEND rather than an atomic rename (which
// would lose prior entries). The caller is responsible for serializing
// concurrent writers via an external lock.
func AppendHistory(projectDir string, entry HistoryEntry) error {
	path := historyPath(projectDir)

	if err := rotateIfNeeded(path, historyBackupPath(projectDir)); err != nil {
		return err
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal history entry: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, atomicFileMode)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotInitialized
		}

		return fmt.Errorf("open history log %s: %w", path, err)
	}

	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write history entry: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync history log: %w", err)
	}

	return nil
}

// rotateIfNeeded renames path to backup when path's size meets or exceeds
// historyRotateBytes. A missing log file is a no-op (first append).
func rotateIfNeeded(path, backup string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("stat history log %s: %w", path, err)
	}

	if info.Size() < historyRotateBytes {
		return nil
	}

	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("rotate %s to %s: %w", path, backup, err)
	}

	return nil
}
