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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/datarobot/cli/tui"
	"golang.org/x/term"
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

// streamSpinnerFrames are spinner.Dot's frames, so a streamed phase's header
// animates exactly like the spinner every other phase shows.
var streamSpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// stream executes one phase whose progress arrives as text lines rather than
// a spinner. The label prints first as a header — animated in place on the
// terminal, so it moves like every other phase's spinner — fn's lines print
// styled and indented beneath it as they arrive, and the check-marked line
// with the elapsed time prints after. The animation repaints only the header
// row, never the cursor's own line, so it cannot fight with the stream.
//
// On the terminal, every line is truncated to the width so it occupies
// exactly one row, and a phase that SUCCEEDS erases its stream — header
// included — leaving only the check-marked line, so a deploy's final
// transcript stays clean. One row per line is what makes the erase count
// exact; a resize mid-phase can still rewrap history, which at worst strands
// a few lines. A failed phase keeps every line: they are the context for the
// error. On a plain writer (no TTY, JSON mode) nothing is truncated or
// erased, which is how CI logs stay complete.
func (r *reporter) stream(label string, fn func(say func(line string, style lipgloss.Style)) error) error {
	started := phaseClock()

	fmt.Fprintf(r.out, "  %s %s\n", tui.HintStyle.Render("⠿"), label)

	// r.spinner is the "out is the terminal the user is watching" flag; a
	// width is only meaningful, and cursor games only safe, on that terminal.
	width := 0
	if r.spinner {
		width = streamWidth()
	}

	var mu sync.Mutex

	rows := 0

	// repaintHeader rewrites the header row, which sits rows+1 rows above the
	// cursor — exact because every streamed line occupies one row. The caller
	// holds mu.
	repaintHeader := func(glyph string) {
		fmt.Fprintf(r.out, "\x1b[%dA\r  %s %s\x1b[K\x1b[%dB\r", rows+1, glyph, label, rows+1)
	}

	stop := make(chan struct{})

	var animator sync.WaitGroup

	if width > 0 {
		animator.Add(1)

		go func() {
			defer animator.Done()

			animateStreamHeader(stop, &mu, repaintHeader)
		}()
	}

	say := func(line string, style lipgloss.Style) {
		mu.Lock()
		defer mu.Unlock()

		// A message with newlines would break the one-row-per-line math.
		for _, part := range strings.Split(line, "\n") {
			if width > 0 {
				part = ansi.Truncate(part, max(width-6, 8), "…")
			}

			fmt.Fprintf(r.out, "    %s\n", style.Render(part))

			rows++
		}
	}

	err := fn(say)

	if width > 0 {
		close(stop)
		animator.Wait()
	}

	if err != nil {
		// A dead animation frame must not outlive the phase; the static
		// glyph says "this phase stopped here" above the kept lines.
		if width > 0 {
			repaintHeader(tui.HintStyle.Render("⠿"))
		}

		return err
	}

	if width > 0 {
		// Cursor up over the stream and its header, clear to the end of the
		// screen; the check-marked line below replaces them.
		fmt.Fprintf(r.out, "\x1b[%dA\x1b[0J", rows+1)
	}

	r.done(label, phaseClock().Sub(started))

	return nil
}

// animateStreamHeader cycles the header glyph until stop closes, locking mu
// around each repaint so frames never interleave with streamed lines.
func animateStreamHeader(stop <-chan struct{}, mu *sync.Mutex, repaint func(glyph string)) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			mu.Lock()
			repaint(tui.InfoStyle.Render(streamSpinnerFrames[frame%len(streamSpinnerFrames)]))
			mu.Unlock()

			frame++
		}
	}
}

// streamWidth returns the width of the terminal the stream renders on, 0 when
// there is none. A var so tests can exercise the terminal path without one.
var streamWidth = func() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 0
	}

	return w
}

// done prints the completed phase. Durations are truncated to a tenth of a
// second: a deploy is measured in minutes and the extra digits are noise.
func (r *reporter) done(label string, elapsed time.Duration) {
	fmt.Fprintf(r.out, "  %s %s %s\n",
		tui.SuccessStyle.Render("✓"),
		label,
		tui.HintStyle.Render("("+elapsed.Truncate(100*time.Millisecond).String()+")"))
}

// say prints a line that is not a phase, such as a warning or a note.
func (r *reporter) say(format string, args ...any) {
	fmt.Fprintf(r.out, format, args...)
}
