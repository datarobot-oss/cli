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

// Package uidiff turns ordered rows into unified-diff-flavored output: the
// three-line context window git made conventional, collapsing of unchanged
// runs longer than that window, line styling from the CLI's shared palette,
// and a caller-supplied hook that keeps a value its owner refuses to print
// out of the output entirely.
//
// The package knows nothing about what the rows describe. Workloads,
// manifests and their name-keyed lists are the caller's business: the caller
// hands over rows whose text it has already formatted, and a predicate
// saying which paths carry values that must never reach the screen. What
// makes a row an addition, a replacement, context, or unmanaged field is
// likewise the caller's judgment; this package only decides what such a row
// looks like.
package uidiff

// Kind classifies one row of a diff body.
type Kind int

const (
	// Context is a row the caller counts as unchanged. Context rows show
	// within the window of a change and collapse into a summary beyond it.
	Context Kind = iota

	// Add is a value the caller is introducing, rendered with a "+".
	Add

	// Del is a value the caller is replacing, rendered with a "-".
	Del

	// Unmanaged is a row the caller wants visible but does not manage, such
	// as a field the live object carries that the file never names. It
	// renders as context, never as a removal, and the caller's Text carries
	// its marker.
	Unmanaged
)

// Row is one line of a diff body.
type Row struct {
	// Kind selects the marker and style the renderer applies.
	Kind Kind

	// Path identifies what the row describes. It exists for the redaction
	// hook and nothing else; the visible text comes from Text.
	Path string

	// Text is the line's content, already formatted by the caller. For
	// Unmanaged rows the leading marker is part of the Text, since only the
	// caller knows the vocabulary the row needs.
	Text string
}

// Options tunes one Render call. The zero value is ready to use: the context
// window defaults to three and nothing is redacted.
type Options struct {
	// Context is how many unchanged rows each side of a change stay visible.
	// Zero means the default, three. There is no CLI flag for it, by design:
	// a diff window is a rendering convention, not a knob.
	Context int

	// Redact reports whether a path's value must never reach the output. A
	// path it reports true for renders as a placeholder in place of its
	// Text, whatever the row's Kind. Nil means nothing is redacted.
	Redact func(path string) bool
}
