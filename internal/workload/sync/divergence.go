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

	"github.com/datarobot/cli/internal/log"
)

// DivergenceKind names the way a single path diverges between BASE
// (manifest.json) and REMOTE (the server's actual file listing). The three
// values are a fixed vocabulary shared by the stderr prose and the JSON
// rendering, so a script can act on the same distinction the message draws.
type DivergenceKind string

const (
	// DivergenceHashMismatch: the path exists on both sides and the hashes
	// differ — the recorded state no longer describes the server's bytes.
	DivergenceHashMismatch DivergenceKind = "hash_mismatch"

	// DivergenceBaseOnly: BASE records the path but REMOTE does not have it.
	DivergenceBaseOnly DivergenceKind = "base_only"

	// DivergenceRemoteOnly: REMOTE has the path but BASE does not record it.
	DivergenceRemoteOnly DivergenceKind = "remote_only"
)

// Divergence is one path where BASE and REMOTE disagree. It is a diagnostic,
// never an error: the reconciling plan already follows from Diff once the
// real remote is known, and the exit status must not change.
type Divergence struct {
	Path string
	Kind DivergenceKind

	// BaseHash and RemoteHash carry the two sides of the disagreement so a
	// script can identify the mismatch without re-fetching anything. The
	// side a kind does not involve is empty (base_only leaves RemoteHash
	// empty, remote_only leaves BaseHash empty).
	BaseHash   string
	RemoteHash string
}

// detectDivergence compares BASE against REMOTE per path and reports every
// disagreement, distinguishing a hash difference from the two one-sided
// cases. The result is sorted by path so notices listing several paths are
// deterministic across runs.
func detectDivergence(base, remote BaseManifest) []Divergence {
	var out []Divergence

	for path, b := range base {
		r, ok := remote[path]

		switch {
		case !ok:
			out = append(out, Divergence{Path: path, Kind: DivergenceBaseOnly, BaseHash: b.Hash})
		case b.Hash != r.Hash:
			out = append(out, Divergence{
				Path: path, Kind: DivergenceHashMismatch,
				BaseHash: b.Hash, RemoteHash: r.Hash,
			})
		}
	}

	for path, r := range remote {
		if _, ok := base[path]; !ok {
			out = append(out, Divergence{Path: path, Kind: DivergenceRemoteOnly, RemoteHash: r.Hash})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out
}

// maybeDetectDivergence checks BASE's claim about the remote when --verify
// has just fetched the real listing for a non-drifted artifact: there — and
// only there — BASE claims to describe exactly the version fetched, so a
// mismatch is a lie worth reporting. On a drifted artifact the remote is a
// newer version by design, and BASE-vs-REMOTE differences are ordinary
// drift, not findings.
//
// Findings are stored on the engine for the display layer AND logged from
// the phase via log.Warn: the log writes stderr immediately, so the findings
// survive a later phase failing and never touch stdout in JSON mode. The
// prose is bounded at DivergenceNoticeBound entries with an "and N more"
// tail; the structured field on the engine carries every divergence
// regardless.
func maybeDetectDivergence(e *Engine) {
	if e.drifted || !e.opts.Verify {
		return
	}

	e.divergences = detectDivergence(e.base, e.remote)

	for i, d := range e.divergences {
		if i >= DivergenceNoticeBound {
			break
		}

		log.Warn(divergenceNotice(d))
	}

	if len(e.divergences) > DivergenceNoticeBound {
		log.Warn(fmt.Sprintf(
			"divergence: and %d more divergence(s) were found (see the plan JSON for the full list)",
			len(e.divergences)-DivergenceNoticeBound))
	}
}

// DivergenceNoticeBound is the maximum number of divergences the stderr prose
// lists individually before summarizing the remainder as a count. The plan
// JSON always lists every divergence. Fixed at 5 so workers and validators
// assert the same number rather than each choosing a bound.
const DivergenceNoticeBound = 5

// divergenceNotice renders one divergence as a self-contained stderr line.
// Each kind gets its own wording: a reader must be able to tell "the server
// holds different bytes" apart from "the path exists on one side only".
func divergenceNotice(d Divergence) string {
	switch d.Kind {
	case DivergenceHashMismatch:
		return fmt.Sprintf(
			"divergence: %s: BASE (manifest.json) hash %s differs from REMOTE (server) hash %s; the manifest no longer describes the server",
			d.Path, d.BaseHash, d.RemoteHash)
	case DivergenceBaseOnly:
		return fmt.Sprintf(
			"divergence: %s: present in BASE (manifest.json) but absent from REMOTE (server)", d.Path)
	case DivergenceRemoteOnly:
		return fmt.Sprintf(
			"divergence: %s: present in REMOTE (server) but absent from BASE (manifest.json)", d.Path)
	}

	return fmt.Sprintf("divergence: %s: unknown kind %q", d.Path, d.Kind)
}
