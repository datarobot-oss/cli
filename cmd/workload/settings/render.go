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
	"strconv"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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

// memoryUnits are the 1000-based suffixes a manifest spells memory in,
// largest first. Decimal on purpose: the platform reads MB as a million
// bytes, and a binary size such as 2 GiB is left as its bare byte count
// rather than rounded to a smaller decimal one.
var memoryUnits = []struct {
	suffix string
	scale  int64
}{
	{"TB", 1_000_000_000_000},
	{"GB", 1_000_000_000},
	{"MB", 1_000_000},
	{"KB", 1_000},
}

// memoryCell spells a byte count the way a manifest would: the largest
// unit that divides it exactly, or the bare count when none does.
func memoryCell(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}

	for _, unit := range memoryUnits {
		if bytes >= unit.scale && bytes%unit.scale == 0 {
			return strconv.FormatInt(bytes/unit.scale, 10) + unit.suffix
		}
	}

	return strconv.FormatInt(bytes, 10)
}
