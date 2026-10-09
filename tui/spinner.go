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

package tui

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/misc/reader"
)

// Noter appends a live suffix to a running spinner's label, replacing whatever
// the last call set. RunWithSpinnerNote hands one to the function it runs,
// and it is safe to call from that function's goroutine while the spinner
// draws. It is a no-op when there is no spinner drawing, so a caller can
// install one unconditionally.
type Noter func(string)

type spinnerModel struct {
	spinner Loading
	label   string
	prefix  string
	fn      func() error
	done    bool

	// styles are bound to the stream the spinner draws on; nil falls back to
	// the package styles, which follow stdout.
	styles *WriterStyles

	// err is what fn returned, carried here by spinnerDoneMsg rather than
	// assigned to a variable the caller closes over. The caller reads it off
	// the final model, which is the only way to read it with a
	// happens-before edge: a captured variable is written by the goroutine
	// running fn and read after Program.Run returns, and on Ctrl-C that read
	// wins the race and sees a nil nobody wrote.
	err error

	// note is the live label suffix, shared with the worker. A pointer so
	// the value survives Bubble Tea copying the model on every Update, and
	// atomic because the worker writes it while the render loop reads it.
	note *atomic.Pointer[string]
}

type spinnerDoneMsg struct{ err error }

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Init(),
		func() tea.Msg {
			return spinnerDoneMsg{err: m.fn()}
		},
	)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerDoneMsg:
		m.done = true
		m.err = msg.err

		return m, tea.Quit
	default:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}

	label := m.label
	info, hint := InfoStyle, HintStyle

	if m.styles != nil {
		info, hint = m.styles.Info, m.styles.Hint
	}

	// A zero-valued model has no note; only RunWithSpinnerNote installs one.
	if m.note != nil {
		if note := m.note.Load(); note != nil && *note != "" {
			label += " " + hint.Render("("+*note+")")
		}
	}

	// The Dot frames carry their own trailing space, so nothing more goes
	// between glyph and label: one column, the same gap the glyph-led lines
	// around a spinner (a phase list's "✓ label") leave.
	return m.prefix + info.Render(m.spinner.View()) + label + "\n"
}

// RunWithSpinner runs fn in the background while showing an animated spinner
// with the given label. Returns the error from fn, if any.
func RunWithSpinner(label string, fn func() error) error {
	return RunWithSpinnerPrefix("", label, fn)
}

// RunWithSpinnerPrefix is RunWithSpinner with a fixed prefix ahead of the
// glyph on every frame, including the non-interactive fallback line. The
// glyph leads the line, so a caller aligning the spinner under other
// glyph-led output (a phase list's "  ✓ label") has to hand the indent in
// here; an indent on the label could never move the glyph.
func RunWithSpinnerPrefix(prefix, label string, fn func() error) error {
	return RunWithSpinnerNote(prefix, label, func(Noter) error { return fn() })
}

// RunWithSpinnerNote runs fn while a spinner draws, and returns what fn
// returned, or ErrInterrupted when the user stopped waiting first.
//
// The cancellation contract: an interrupt (Ctrl-C as a keystroke, or SIGINT
// reaching the process) ends the spinner and this call, but not fn. Bubble
// Tea leaks its Cmd goroutines by design, and nothing here can stop a
// function it was handed, so fn keeps running until it returns on its own.
// A caller whose fn does anything long-lived, such as polling a remote
// service, should give it a context it cancels when this returns, which is
// what the deploy's phase runner does. ErrInterrupted is never nil and never
// fn's own result: an abandoned fn has no result yet, and reporting that
// absence as success is the mistake this is built to avoid.
//
// fn is handed a Noter that rewrites the parenthesised suffix after the
// label, so a phase that knows how it is progressing can say so while it
// runs rather than only once it ends. Without a spinner on screen (no
// terminal, or a non-interactive run) fn runs inline, the Noter does
// nothing, and interrupts are whatever fn's own context makes of them.
func RunWithSpinnerNote(prefix, label string, fn func(note Noter) error) error {
	// Colors follow stderr, where the spinner and its fallback line go, not
	// stdout, which the package styles probe.
	styles := StylesFor(os.Stderr)

	switch spinnerModeFor(reader.IsStdinTerminal(), reader.IsStderrTerminal(), reader.IsNonInteractive()) {
	case spinnerSilent:
		return fn(func(string) {})
	case spinnerLine:
		fmt.Fprintln(os.Stderr, prefix+styles.Info.Render("• ")+label)

		return fn(func(string) {})
	case spinnerDrawn:
	}

	note := &atomic.Pointer[string]{}

	m := spinnerModel{
		spinner: NewLoading(),
		label:   label,
		prefix:  prefix,
		note:    note,
		styles:  &styles,
		fn: func() error {
			return fn(func(s string) { note.Store(&s) })
		},
	}
	m.spinner.Spinner.Style = styles.Info

	// On stderr, never stdout: stdout is the command's data, and a frame drawn
	// there ends up in front of it whenever stdout is redirected.
	return spinnerVerdict(Run(m, tea.WithOutput(os.Stderr)))
}

type spinnerMode int

const (
	// spinnerSilent runs fn with nothing shown: no terminal to answer
	// keystrokes, so nothing is drawn.
	spinnerSilent spinnerMode = iota
	// spinnerLine prints the label once, for a run that must not animate or
	// whose stderr is not a terminal.
	spinnerLine
	// spinnerDrawn animates the spinner on stderr.
	spinnerDrawn
)

// SpinnerDrawn reports whether a spinner would animate on stderr, for a
// caller that must show its label some other way when it would not.
func SpinnerDrawn(stdinTerm, stderrTerm, nonInteractive bool) bool {
	return spinnerModeFor(stdinTerm, stderrTerm, nonInteractive) == spinnerDrawn
}

// spinnerModeFor decides how a spinner shows: animated only when both the
// keyboard and the screen it draws on are a terminal.
func spinnerModeFor(stdinTerm, stderrTerm, nonInteractive bool) spinnerMode {
	switch {
	case !stdinTerm:
		return spinnerSilent
	case nonInteractive || !stderrTerm:
		return spinnerLine
	default:
		return spinnerDrawn
	}
}

// spinnerVerdict turns what Run handed back (the final model and its error)
// into the answer RunWithSpinnerNote gives: the run's error, ErrInterrupted
// for either kind of interrupt, or otherwise what fn returned. It is the one
// read that decides whether an abandoned call counts as a finished one, so it
// lives apart from the terminal it needs, where a test can hold every branch;
// the suite never has a TTY, so inside RunWithSpinnerNote none of this would
// be exercised.
func spinnerVerdict(final tea.Model, runErr error) error {
	if runErr != nil {
		// Bubble Tea answers a SIGINT that reaches the process while it is
		// drawing with an error of its own, distinct from the keystroke the
		// wrapper catches. Both mean the user stopped waiting, and a caller
		// checking for ErrInterrupted has to see them as one thing, or an
		// external `kill -INT` skips the explanation the keystroke gets.
		if errors.Is(runErr, tea.ErrInterrupted) {
			return ErrInterrupted
		}

		return runErr
	}

	// Asked before the model is read, because an interrupted run quits with
	// fn still in flight: the model carries no error because there is no
	// answer yet, and reporting that absence as success is the bug.
	if WasInterrupted(final) {
		return ErrInterrupted
	}

	done, ok := Unwrap(final).(spinnerModel)
	if !ok {
		// Never nil. Falling through to a nil here would be the same defect
		// this function exists to fix: an error that reads as success because
		// it was not where it was looked for. The chain Run() wraps this in
		// (InterruptibleModel, then the Konami overlay) is peeled by Unwrap,
		// so reaching this means a wrapper was added that does not implement
		// unwrapper — a programming error, and one that would otherwise turn
		// every failed phase in the CLI into a silent success.
		return fmt.Errorf("spinner finished but its model was lost behind %T; the phase's own result is unknown", final)
	}

	return done.err
}
