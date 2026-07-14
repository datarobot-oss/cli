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

import tea "github.com/charmbracelet/bubbletea"

// unwrapper is implemented by any tui wrapper model (InterruptibleModel,
// sequenceOverlay, and any future wrapper) that holds another model inside
// it.
type unwrapper interface {
	Unwrap() tea.Model
}

// Unwrap peels off every wrapper layer tui.Run() adds (InterruptibleModel
// for Ctrl-C handling, the Konami sequenceOverlay, etc.) and returns the
// original concrete model passed into tui.Run().
//
// Callers that type-assert a tui.Run() result down to their own concrete
// model type — e.g. to read a field set during Update() — must call this
// first instead of asserting straight to tui.InterruptibleModel: tui.Run()
// wraps every model in additional layers, and a caller that only unwraps
// InterruptibleModel will get a different wrapper type back instead of its
// own model, causing the type assertion to silently fail.
func Unwrap(m tea.Model) tea.Model {
	for {
		u, ok := m.(unwrapper)
		if !ok {
			return m
		}

		m = u.Unwrap()
	}
}
