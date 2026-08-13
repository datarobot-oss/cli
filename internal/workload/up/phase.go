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

import (
	"fmt"
	"io"
	"time"

	"github.com/datarobot/cli/tui"
)

// phaseClock is the time source, swapped by tests so a check-marked line's
// duration is deterministic rather than however long the machine took.
var phaseClock = time.Now

// reporter prints a deploy's progress. Every line goes to the writer it is
// given rather than to os.Stderr, which is what lets a command test capture
// them and what keeps them off stdout when the caller is emitting JSON.
//
// tui.RunWithSpinner writes to os.Stderr directly, so the spinner is only
// worth using when the writer is the process's real stderr. Below that it
// would draw somewhere nobody is looking while the captured stream stayed
// empty.
type reporter struct {
	out     io.Writer
	spinner bool
}

// newReporter builds a reporter for out. spinner should be true only when out
// is the terminal the user is watching.
func newReporter(out io.Writer, spinner bool) *reporter {
	return &reporter{out: out, spinner: spinner}
}

// run executes one phase, announcing it while it works and check-marking it
// with its elapsed time when it succeeds. A failure prints nothing extra: the
// error carries the story, and a checkmark followed by an error message reads
// as though both happened.
func (r *reporter) run(label string, fn func() error) error {
	started := phaseClock()

	if err := r.work(label, fn); err != nil {
		return err
	}

	r.done(label, phaseClock().Sub(started))

	return nil
}

// work performs the phase, with a spinner when there is a terminal to spin on.
func (r *reporter) work(label string, fn func() error) error {
	if r.spinner {
		return tui.RunWithSpinner(label, fn)
	}

	return fn()
}

// done prints the completed phase. Durations are truncated to a tenth of a
// second: a deploy is measured in minutes and the extra digits are noise.
func (r *reporter) done(label string, elapsed time.Duration) {
	fmt.Fprintf(r.out, "  ✓ %s (%s)\n", label, elapsed.Truncate(100*time.Millisecond))
}

// say prints a line that is not a phase, such as a warning or a note.
func (r *reporter) say(format string, args ...any) {
	fmt.Fprintf(r.out, format, args...)
}
