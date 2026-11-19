// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fbiville/markdown-table-formatter/pkg/markdown"
)

// mdformat is a simple CLI tool to format Markdown files, with a focus on aligning tables.
// It is inspired by Obsidian.
func main() {
	checkMode := flag.Bool("check", false, "check mode: exit with code 1 if files need formatting")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: mdformat [-check] <file_or_dir> ...\n")
		os.Exit(1)
	}

	var exitCode int
	for _, arg := range args {
		if err := processPath(arg, *checkMode); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func processPath(path string, checkMode bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if info.IsDir() {
		return filepath.Walk(path, func(filePath string, fileInfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip .factory and node_modules directories
			if fileInfo.IsDir() && (fileInfo.Name() == ".factory" || fileInfo.Name() == "node_modules") {
				return filepath.SkipDir
			}

			if !fileInfo.IsDir() && strings.HasSuffix(fileInfo.Name(), ".md") {
				return processFile(filePath, checkMode)
			}
			return nil
		})
	}

	return processFile(path, checkMode)
}

func processFile(filePath string, checkMode bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	originalContent := string(content)
	formatted := formatMarkdown(originalContent)

	if checkMode {
		if originalContent != formatted {
			fmt.Printf("%s needs formatting\n", filePath)
			return fmt.Errorf("file needs formatting")
		}
		return nil
	}

	if originalContent != formatted {
		if err := os.WriteFile(filePath, []byte(formatted), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", filePath, err)
		}
		fmt.Printf("%s formatted\n", filePath)
	}

	return nil
}

func formatMarkdown(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var result strings.Builder
	var tableLines []string
	inFence := false

	// flush writes out a pending line group: as a formatted table only when it
	// really is one (a separator row directly under the header), verbatim
	// otherwise, so prose that merely contains pipes is never rearranged.
	flush := func() {
		if len(tableLines) > 1 && isSeparatorRow(parseCells(tableLines[1])) {
			for _, fmtLine := range formatTable(tableLines) {
				result.WriteString(fmtLine)
				result.WriteString("\n")
			}
		} else {
			for _, line := range tableLines {
				result.WriteString(line)
				result.WriteString("\n")
			}
		}

		tableLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Fenced code is opaque: pipes there are shell pipelines, literals,
		// or ASCII layouts, never table syntax this tool may rewrite.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flush()
			inFence = !inFence
			result.WriteString(line)
			result.WriteString("\n")

			continue
		}

		if !inFence && isTableLine(line) {
			tableLines = append(tableLines, line)

			continue
		}

		flush()
		result.WriteString(line)
		result.WriteString("\n")
	}

	flush()

	return result.String()
}

// isTableLine reports whether the line could belong to a table: GitHub table
// syntax starts the row with a pipe, and everything else with a pipe in it
// (prose, bullet text, inline command alternatives) is left alone.
func isTableLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

func formatTable(lines []string) []string {
	if len(lines) < 2 {
		return lines
	}

	var headers []string
	var rows [][]string

	for i, line := range lines {
		cells := parseCells(line)

		// First row is headers
		if i == 0 {
			headers = cells
			continue
		}

		// Skip separator row
		if isSeparatorRow(cells) {
			continue
		}

		// Rest are data rows
		rows = append(rows, cells)
	}

	if len(headers) == 0 {
		return lines
	}

	// Use fbiville library to format with pretty printing
	table, err := markdown.NewTableFormatterBuilder().
		WithPrettyPrint().
		Build(headers...).
		Format(rows)
	if err != nil {
		// Fall back to original lines if formatting fails
		return lines
	}

	return strings.Split(strings.TrimSpace(table), "\n")
}

func parseCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")

	parts := strings.Split(trimmed, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}

	return cells
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}

	for _, cell := range cells {
		trimmed := strings.TrimSpace(cell)
		if !isSeparatorCell(trimmed) {
			return false
		}
	}

	return true
}

func isSeparatorCell(cell string) bool {
	if len(cell) == 0 {
		return false
	}

	for _, ch := range cell {
		if ch != ':' && ch != '-' {
			return false
		}
	}

	return true
}
