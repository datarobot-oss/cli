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

package settings

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// newTable is the package's one table shape, styled like every other
// listing in the CLI.
func newTable(headers ...string) *table.Table {
	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			return cellStyle
		}).
		Headers(headers...)
}

// memoryCell spells a byte count the way a manifest would, which the
// manifest package decides: the largest decimal unit that divides it
// exactly, or the bare count when none does, so 2 GiB is never rounded to
// a smaller 2GB.
func memoryCell(bytes int64) string {
	if spelled := manifest.MemoryString(bytes); spelled != "" {
		return spelled
	}

	return "-"
}
