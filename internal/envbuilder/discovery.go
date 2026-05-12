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

package envbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/datarobot/cli/internal/log"
)

// depth gets our current directory depth by file path
func depth(path string) int {
	// Windows uses backslashes, so we normalize to forward slashes
	normalized := filepath.ToSlash(path)
	if normalized == "." {
		return 0
	}

	// +1 to count the root directory itself
	return strings.Count(normalized, "/") + 1
}

func Discover(root string, maxDepth int) ([]string, error) {
	includes, err := findComponents(filepath.Join(root, ".datarobot"), maxDepth)
	if err != nil {
		return []string{""}, fmt.Errorf("Failed to discover components: %w", err)
	}

	if len(includes) == 0 {
		return []string{""}, nil
	}

	return includes, nil
}

// shouldSkipDir checks if a directory should be skipped during file walk.
// Content-based filtering of individual files happens downstream in
// filePrompts via shape detection; this function only prunes directories
// that we never want to descend into.
func shouldSkipDir(name string, currentDepth, maxDepth int) bool {
	if currentDepth > maxDepth {
		return true
	}

	if strings.HasPrefix(name, ".") && name != "." && name != ".datarobot" {
		return true
	}

	return false
}

// findComponents looks for the *.{yaml,yml} files in subdirectories (e.g. which are app framework components) of the given .datarobot directory,
// and returns discovered components
func findComponents(root string, maxDepth int) ([]string, error) {
	var includes []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Debug(err)
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			log.Debug(err)
			return nil
		}

		currentDepth := depth(relPath)
		if shouldSkipDir(name, currentDepth, maxDepth) {
			return filepath.SkipDir
		}

		matches, err := filepath.Glob(filepath.Join(path, "*.y*ml"))
		if err != nil {
			log.Debug(err)
			return nil
		}

		includes = append(includes, matches...)

		return nil
	})

	// sort the list to make the order consistent
	sort.Slice(includes, func(i, j int) bool {
		return includes[i] < includes[j]
	})

	return includes, err
}
