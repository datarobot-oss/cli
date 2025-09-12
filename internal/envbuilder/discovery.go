// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
)

// depth gets our current directory depth by file path
func depth(path string) int {
	if path == "." {
		return 0
	}

	// +1 to count the root directory itself
	return strings.Count(path, "/") + 1
}

func Discover(root string, maxDepth int) ([]string, error) {
	includes, err := findComponents(filepath.Join(root, ".datarobot"), maxDepth)
	if err != nil {
		return []string{""}, fmt.Errorf("failed to discover components: %w", err)
	}

	if len(includes) == 0 {
		return []string{""}, nil
	}

	return includes, nil
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

		name := strings.ToLower(info.Name())

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			log.Debug(err)
			return nil
		}

		currentDepth := depth(relPath)

		if info.IsDir() {
			if (strings.HasPrefix(name, ".") && name != "." && name != ".datarobot") || currentDepth > maxDepth {
				// skip all hidden dirs (except for our root dir) or if we have already dived too deep
				return filepath.SkipDir
			}
		}

		matches, err := filepath.Glob(filepath.Join(path, "*.y*ml"))
		if err != nil {
			log.Debug(err)
			return nil
		}

		if len(matches) == 0 {
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
