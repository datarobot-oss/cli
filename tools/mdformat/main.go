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
	inTable := false

	for scanner.Scan() {
		line := scanner.Text()

		if isTableLine(line) {
			if !inTable {
				inTable = true
			}
			tableLines = append(tableLines, line)
		} else {
			if inTable && len(tableLines) > 0 {
				formatted := formatTable(tableLines)
				for _, fmtLine := range formatted {
					result.WriteString(fmtLine)
					result.WriteString("\n")
				}
				tableLines = nil
				inTable = false
			}
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	if inTable && len(tableLines) > 0 {
		formatted := formatTable(tableLines)
		for _, fmtLine := range formatted {
			result.WriteString(fmtLine)
			result.WriteString("\n")
		}
	}

	return result.String()
}

func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, "|")
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
