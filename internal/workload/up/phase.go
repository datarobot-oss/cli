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
	"github.com/datarobot/cli/internal/log"
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
// The two-space prefix keeps the animated glyph in the column where stream's
// ⠿ header and done's ✓ sit.
func (r *reporter) work(label string, fn func() error) error {
	if r.spinner {
		return tui.RunWithSpinnerPrefix("  ", label, fn)
	}

	return fn()
}

// streamSpinnerFrames are spinner.Dot's frames, so a streamed phase's header
// animates exactly like the spinner every other phase shows.
var streamSpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// streamWindowRows caps the live region at header + this many lines: tall
// enough to follow a build's parallel steps, small enough to redraw cheaply
// and fit a viewport (shorter terminals shrink it further).
const streamWindowRows = 20

// stream executes one phase whose progress arrives as text lines rather than
// a spinner. The label prints first as a header — animated in place on the
// terminal, so it moves like every other phase's spinner — and fn's lines
// render beneath it as they arrive, with the check-marked line and elapsed
// time printing after.
//
// On the terminal, the lines live in a fixed-height sliding window (the way
// docker buildx renders): only the newest lines stay on screen, each
// truncated to the width so it occupies exactly one row, and every new line
// redraws the region in place. Nothing ever scrolls, which is what makes the
// two cursor tricks safe — the cursor cannot move above the viewport, so a
// region taller than the screen could neither be animated nor erased, and
// scrolled-away rows would survive in scrollback where no escape code
// reaches. A phase that SUCCEEDS erases the window — header included —
// leaving only the check-marked line, so the deploy's final transcript stays
// clean. A failed phase keeps the window's tail: the freshest context for
// the error, with the full log a command away. On a plain writer (no TTY,
// JSON mode) every line prints permanently, which is how CI logs stay
// complete.
func (r *reporter) stream(label string, fn func(say func(line string, style lipgloss.Style)) error) error {
	started := phaseClock()

	fmt.Fprintf(r.out, "  %s %s\n", tui.HintStyle.Render("⠿"), label)

	// r.spinner is the "out is the terminal the user is watching" flag; a
	// live region is only safe on that terminal, so anywhere else lines just
	// print permanently.
	var region *liveRegion
	if r.spinner {
		region = newLiveRegion(r.out, label)
	}

	stop := make(chan struct{})

	var (
		animator sync.WaitGroup
		haltOnce sync.Once
	)

	// halt stops the animation exactly once. It is deferred so a panic inside
	// fn cannot unwind past a still-ticking animator, and also called inline
	// on both exits, because finishSuccess and finishFailure repaint rows the
	// animator writes to and must not race it.
	halt := func() {
		if region == nil {
			return
		}

		haltOnce.Do(func() {
			close(stop)
			animator.Wait()
		})
	}

	defer halt()

	if region != nil {
		animator.Add(1)

		go func() {
			defer animator.Done()

			// The animation is cosmetic; a panic here must not take the
			// deploy down mid-build. Recovered into the debug log rather
			// than swallowed, so it stays findable.
			defer func() {
				if rec := recover(); rec != nil {
					log.Debug("stream header animator panicked", "panic", rec)
				}
			}()

			animateStreamHeader(stop, region)
		}()
	}

	say := func(line string, style lipgloss.Style) {
		// A message with newlines would break the one-row-per-line math.
		for _, part := range strings.Split(line, "\n") {
			if region == nil {
				fmt.Fprintf(r.out, "    %s\n", style.Render(part))
			} else {
				region.append(part, style)
			}
		}
	}

	err := fn(say)

	halt()

	if err != nil {
		if region != nil {
			region.finishFailure()
		}

		return err
	}

	if region != nil {
		region.finishSuccess()
	}

	r.done(label, phaseClock().Sub(started))

	return nil
}

// liveRegion is the fixed-height window a streamed phase renders into: the
// header row plus up to window lines, redrawn in place so nothing ever
// scrolls. All methods lock internally; the animator and the stream share it.
type liveRegion struct {
	out    io.Writer
	label  string
	width  int
	window int

	mu    sync.Mutex
	tail  []string // rendered lines on screen, newest last
	drawn int      // rows currently on screen below the header
}

// newLiveRegion sizes a region for the terminal, nil when there is none. The
// window must fit the viewport with the header and a little slack: the cursor
// cannot move above the viewport, so a taller region could neither be redrawn
// nor erased, and scrolled-away rows would survive in scrollback.
func newLiveRegion(out io.Writer, label string) *liveRegion {
	width, height := streamSize()
	if width <= 0 {
		return nil
	}

	return &liveRegion{
		out:    out,
		label:  label,
		width:  width,
		window: min(streamWindowRows, max(height-4, 1)),
	}
}

// append truncates the line to one row, slides the window, and repaints it.
func (l *liveRegion) append(line string, style lipgloss.Style) {
	l.mu.Lock()
	defer l.mu.Unlock()

	line = ansi.Truncate(sanitizeRow(line), max(l.width-6, 8), "…")

	l.tail = append(l.tail, "    "+style.Render(line))
	if len(l.tail) > l.window {
		l.tail = l.tail[len(l.tail)-l.window:]
	}

	if l.drawn > 0 {
		fmt.Fprintf(l.out, "\x1b[%dA", l.drawn)
	}

	for _, row := range l.tail {
		fmt.Fprintf(l.out, "\r%s\x1b[K\n", row)
	}

	l.drawn = len(l.tail)
}

// sanitizeRow rewrites the control characters the region's row math cannot
// survive. ansi.Truncate counts a tab as zero columns while the terminal
// renders it, so a near-width line carrying one comes out wider than the
// row and wraps, putting every redraw after it off by a row; a bare CR sends
// the cursor back to column 0, so the \x1b[K meant for the line's tail
// blanks the row instead. ESC stays: build output carries its own colors,
// and Truncate counts those correctly.
func sanitizeRow(line string) string {
	if !strings.ContainsFunc(line, isRowControl) {
		return line
	}

	var b strings.Builder

	b.Grow(len(line))

	for _, r := range line {
		switch {
		case r == '\t':
			// The four columns a terminal gives an unlucky tab, in spaces
			// the width math can count.
			b.WriteString("    ")
		case isRowControl(r):
			// Dropped: nothing a one-row window can render.
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// isRowControl reports a control character other than ESC, which is the one
// with a legitimate job on a rendered row.
func isRowControl(r rune) bool {
	return (r < 0x20 || r == 0x7f) && r != '\x1b'
}

// repaintHeader rewrites the header row, drawn+1 rows above the cursor —
// exact because the window's lines each occupy one row.
func (l *liveRegion) repaintHeader(glyph string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Fprintf(l.out, "\x1b[%dA\r  %s %s\x1b[K\x1b[%dB\r", l.drawn+1, glyph, l.label, l.drawn+1)
}

// finishSuccess erases the window and its header; the caller's check-marked
// line replaces them.
func (l *liveRegion) finishSuccess() {
	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Fprintf(l.out, "\x1b[%dA\x1b[0J", l.drawn+1)
}

// finishFailure repaints the header static, so a dead animation frame does
// not outlive the phase above the kept tail — the freshest context for the
// error, with the full log a command away.
func (l *liveRegion) finishFailure() {
	l.repaintHeader(tui.HintStyle.Render("⠿"))
}

// animateStreamHeader cycles the region's header glyph until stop closes; the
// region's own lock keeps frames from interleaving with streamed lines.
func animateStreamHeader(stop <-chan struct{}, region *liveRegion) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			region.repaintHeader(tui.InfoStyle.Render(streamSpinnerFrames[frame%len(streamSpinnerFrames)]))

			frame++
		}
	}
}

// streamSize returns the (width, height) of the terminal the stream renders
// on, zeros when there is none. A var so tests can exercise the terminal path
// without one.
var streamSize = func() (int, int) {
	w, h, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0
	}

	return w, h
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
