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
	"fmt"
	"os"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/misc/reader"
)

// Noter appends a live suffix to a running spinner's label, replacing whatever
// the last call set. It is safe to call from the worker goroutine, and it is a
// no-op when there is no spinner drawing, so a caller can install one
// unconditionally.
type Noter func(string)

type spinnerModel struct {
	spinner Loading
	label   string
	prefix  string
	fn      func() error
	done    bool

	// err is what fn returned, carried here by spinnerDoneMsg rather than
	// assigned to a variable the caller closes over. The caller reads it off
	// the final model, which is the only way to read it with a
	// happens-before edge: a captured variable is written by the goroutine
	// running fn and read after Program.Run returns, and on Ctrl-C that read
	// wins the race and sees a nil nobody wrote (RAPTOR-19963).
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

	// A zero-valued model has no note; only RunWithSpinnerNote installs one.
	if m.note != nil {
		if note := m.note.Load(); note != nil && *note != "" {
			label += " " + HintStyle.Render("("+*note+")")
		}
	}

	// The Dot frames carry their own trailing space, so nothing more goes
	// between glyph and label: one column, the same gap the glyph-led lines
	// around a spinner (a phase list's "✓ label") leave.
	return m.prefix + InfoStyle.Render(m.spinner.View()) + label + "\n"
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

// RunWithSpinnerNote is RunWithSpinnerPrefix with a live label. fn is handed a
// Noter that rewrites the parenthesised suffix after the label, so a phase
// that knows how it is progressing can say so while it runs rather than only
// once it ends. Without a spinner on screen the Noter does nothing: the
// caller is expected to have its own way of reporting progress to a plain
// stream, and a spinner's job is the terminal.
//
// An interrupt comes back as ErrInterrupted. fn keeps running — Bubble Tea
// leaks its Cmd goroutines by design and nothing here can stop somebody
// else's function — so a caller whose fn polls a remote thing should give it
// a context it cancels when this returns.
func RunWithSpinnerNote(prefix, label string, fn func(note Noter) error) error {
	if !reader.IsStdinTerminal() {
		return fn(func(string) {})
	}

	if reader.IsNonInteractive() {
		fmt.Fprintln(os.Stderr, prefix+InfoStyle.Render("• ")+label)

		return fn(func(string) {})
	}

	note := &atomic.Pointer[string]{}

	m := spinnerModel{
		spinner: NewLoading(),
		label:   label,
		prefix:  prefix,
		note:    note,
		fn: func() error {
			return fn(func(s string) { note.Store(&s) })
		},
	}

	final, err := Run(m)
	if err != nil {
		return err
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
