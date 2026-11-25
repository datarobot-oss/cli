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

package sync

import (
	"fmt"
	"sort"
	"strings"

	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// executableNoticeMaxNames caps the file list so a tree of scripts stays one line.
const executableNoticeMaxNames = 5

// mergeExecutable is the bit a file ends with, merged apart from its bytes so
// a chmod on one side survives an edit on the other: the side that moved off
// BASE wins. With no BASE, older CLIs only ever dropped the bit, so a known
// executable side wins. Windows has no local side, so the remote's is kept.
// Nil when nothing is known; the file then goes as a plain one.
func mergeExecutable(base, local, remote *bool) *bool {
	if base == nil {
		if isSet(local) || isSet(remote) {
			return boolPtr(true)
		}

		if local != nil {
			return local
		}

		return remote
	}

	switch {
	case local != nil && *local != *base:
		return local
	case remote != nil && *remote != *base:
		return remote
	}

	return base
}

// keepLocalExecutable sets an uploaded file's merged bit on disk when the
// other side's chmod won, so the file and BASE agree after the sync and the
// next one does not read the difference as a local chmod.
func keepLocalExecutable(abs string, local, merged *bool) error {
	if local == nil || merged == nil || *local == *merged {
		return nil
	}

	if err := fileops.ApplyExecutable(abs, merged); err != nil {
		return fmt.Errorf("apply the merged executable bit: %w", err)
	}

	return nil
}

// sentExecutable is the bit an upload recorded: nil when the server cannot
// take it, so BASE never claims a bit the catalog was never sent.
func sentExecutable(exec *bool, supported bool) *bool {
	if !supported {
		return nil
	}

	return exec
}

func boolPtr(b bool) *bool { return &b }

func isSet(b *bool) bool {
	return b != nil && *b
}

// hasUnknownExecutable reports a base entry written before the bit was tracked.
func hasUnknownExecutable(base BaseManifest) bool {
	for _, entry := range base {
		if entry.Executable == nil {
			return true
		}
	}

	return false
}

// remoteReportsExecutable reports whether a listing carried the bit at all. A
// server older than API 2.49 sends it for no file; an empty listing says
// nothing either way.
func remoteReportsExecutable(remote RemoteManifest) bool {
	if len(remote) == 0 {
		return true
	}

	for _, entry := range remote {
		if entry.Executable != nil {
			return true
		}
	}

	return false
}

// executableUnreported is the config's mark for a server that reports no
// executable bits: a listing this run answers it, and without one the last
// answer stands.
func executableUnreported(e *Engine, cfg wapi.Config) bool {
	if !e.remoteListed {
		return cfg.ExecutableUnreported
	}

	return !remoteReportsExecutable(e.remote)
}

// persistExecutableBackfill records what a backfill listing learned, so the
// next sync takes the fast path again: the bits of files whose bytes still
// match, except those a push-only run set aside, whose disk copy never got the
// remote's bit; or, when the server reports no bits, that it does not. A run
// that executes a plan writes both in its state phase instead.
func persistExecutableBackfill(e *Engine) error {
	if !e.execBackfill {
		return nil
	}

	if !remoteReportsExecutable(e.remote) {
		cfg := e.config
		cfg.ExecutableUnreported = true

		return wapi.SaveConfig(e.projectDir, cfg)
	}

	m, err := wapi.LoadManifest(e.projectDir)
	if err != nil {
		return err
	}

	if !learnExecutableBits(m.Files, e.remote, e.plan.Skipped) {
		return nil
	}

	return wapi.SaveManifest(e.projectDir, m)
}

// learnExecutableBits fills unknown bits in files from the listing where the
// bytes still match, leaving set-aside paths alone; it reports any change.
func learnExecutableBits(files map[string]wapi.FileMeta, remote RemoteManifest, setAside []FileAction) bool {
	skipped := make(map[string]bool, len(setAside))
	for _, fa := range setAside {
		skipped[fa.Path] = true
	}

	changed := false

	for path, meta := range files {
		r, ok := remote[path]
		if meta.Executable != nil || !ok || skipped[path] || r.Executable == nil || r.Hash != meta.Hash {
			continue
		}

		meta.Executable = r.Executable
		files[path] = meta
		changed = true
	}

	return changed
}

// executableNotice warns that the image build drops the bit the catalog now
// keeps, naming the executable files whose bit this sync brought to the
// catalog, so an edit to a file it already named stays quiet; "" when there
// are none, which includes a server that never received the bit. The
// Dockerfile advice is only for a project that writes its own. Remove it once
// image builds apply the bit.
func executableNotice(sent map[string]FileEntry, before RemoteManifest, generated bool) string {
	var names []string

	for path, entry := range sent {
		if isSet(entry.Executable) && !isSet(before[path].Executable) {
			names = append(names, path)
		}
	}

	if len(names) == 0 {
		return ""
	}

	sort.Strings(names)

	listed := names
	if len(listed) > executableNoticeMaxNames {
		listed = listed[:executableNoticeMaxNames]
	}

	list := strings.Join(listed, ", ")
	if extra := len(names) - len(listed); extra > 0 {
		list += fmt.Sprintf(" and %d more", extra)
	}

	if generated {
		return "Image builds do not keep the executable bit yet, so these are not executable in the image: " + list
	}

	return "Image builds do not keep the executable bit yet; use `COPY --chmod=755` in your Dockerfile for: " + list
}
