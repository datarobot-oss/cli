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

package up

import "io"

// deployMargin is the column the wizard's prose is moved to when a deploy
// borrows it. It is not a single source of truth for the deploy's own output:
// the plan, the progress and the endpoint each spell their margin where they
// print it, and changing this moves only what indented wraps.
const deployMargin = "  "

// indented puts a margin in front of every line another package writes.
//
// The wizard formats for `dr workload config`, where what it says is the whole
// of the run and the left edge is its own. Inside a deploy the same sentences
// are a preamble to a plan that is indented, and a notice flush against the
// margin reads as something that happened before this command rather than as
// part of it.
//
// A blank line keeps its margin off, because a margin with nothing after it is
// trailing whitespace.
type indented struct {
	w      io.Writer
	prefix string

	// atLineStart says the next byte begins a line and so needs the margin.
	// It starts true: the writer is handed over at the start of one.
	atLineStart bool
}

// indent wraps w so every line written through it carries prefix.
func indent(w io.Writer, prefix string) *indented {
	return &indented{w: w, prefix: prefix, atLineStart: true}
}

// Write builds the margined form of p and hands it over in one write, so
// nothing else sharing the stream can land between a line and the one indented
// under it.
//
// It reports len(p) rather than what reached w, which is what io.Writer's
// contract asks of a wrapper that adds bytes of its own: a caller counting
// its own output must not be told it wrote the margin too. A failure claims
// nothing at all, because the two lengths differ and a partial count would
// have to guess which of the caller's bytes got through.
func (i *indented) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	out := make([]byte, 0, len(p)+len(i.prefix))
	atLineStart := i.atLineStart

	for _, b := range p {
		if atLineStart && b != '\n' {
			out = append(out, i.prefix...)
		}

		out = append(out, b)

		atLineStart = b == '\n'
	}

	n, err := i.w.Write(out)

	// A writer that reports a short write without an error has broken its own
	// contract, and passing that on as success would drop the tail of a
	// warning with nothing to say so.
	if err == nil && n < len(out) {
		err = io.ErrShortWrite
	}

	if err != nil {
		// The state describes what w actually received, so a caller that
		// writes again does not put a margin in the middle of the line this
		// one failed to finish.
		i.atLineStart = n > 0 && out[n-1] == '\n'

		return 0, err
	}

	i.atLineStart = atLineStart

	return len(p), nil
}
