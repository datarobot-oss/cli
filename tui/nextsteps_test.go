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
	"bytes"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

// The descriptions line up in one column however long each command is, which
// is what makes a list of several read as a list rather than as prose.
func TestPrintNextSteps_AlignsTheDescriptions(t *testing.T) {
	var buf bytes.Buffer

	PrintNextSteps(&buf,
		NextStep{Command: "dr workload logs", Description: "View the container logs"},
		NextStep{Command: "dr workload status", Description: "Check the workload status"},
	)

	assert.Equal(t, "\nNext:\n"+
		"  dr workload logs    View the container logs\n"+
		"  dr workload status  Check the workload status\n", ansi.Strip(buf.String()))
}

// Color follows the writer rather than stdout, and the padding stays outside
// the styled span, so a terminal that renders the codes shows the same column.
func TestPrintNextSteps_ColorFollowsTheWriter(t *testing.T) {
	t.Run("plain for a writer that is not a terminal", func(t *testing.T) {
		t.Setenv("CLICOLOR_FORCE", "")

		var buf bytes.Buffer

		PrintNextSteps(&buf, NextStep{Command: "dr workload up", Description: "Deploy the workload"})

		assert.Equal(t, "\nNext:\n  dr workload up  Deploy the workload\n", buf.String())
	})

	t.Run("styled where color is forced, with the same layout", func(t *testing.T) {
		t.Setenv("CLICOLOR_FORCE", "1")

		var buf bytes.Buffer

		PrintNextSteps(&buf,
			NextStep{Command: "dr workload logs", Description: "View the container logs"},
			NextStep{Command: "dr workload status", Description: "Check the workload status"},
		)

		assert.NotEqual(t, ansi.Strip(buf.String()), buf.String(), "the block is styled")
		assert.Contains(t, ansi.Strip(buf.String()), "  dr workload logs    View the container logs\n")
	})
}

// A command is copied as printed, so a tab in a quoted path survives rather
// than turning into the spaces of a directory that does not exist.
func TestPrintNextSteps_KeepsTabs(t *testing.T) {
	var buf bytes.Buffer

	PrintNextSteps(&buf, NextStep{Command: "dr workload up --dir \"a\tb\"", Description: "Deploy the workload"})

	assert.Contains(t, ansi.Strip(buf.String()), "dr workload up --dir \"a\tb\"")
}

// A caller whose list came out empty gets no block at all, not a Next: header
// over nothing.
func TestPrintNextSteps_NoStepsPrintsNothing(t *testing.T) {
	var buf bytes.Buffer

	PrintNextSteps(&buf)

	assert.Empty(t, buf.String())
}
