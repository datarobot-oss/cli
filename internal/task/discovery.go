// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package task

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"
)

//go:embed Taskfile.tmpl.yaml
var tmplFS embed.FS

type componentInclude struct {
	Name     string
	Taskfile string
	Dir      string
}

type taskfileTmplData struct {
	Hash     string
	Includes []componentInclude
}

// depth gets our current directory depth by file path
func depth(path string) int {
	if path == "." {
		return 0
	}

	// +1 to count the root directory itself
	return strings.Count(path, "/") + 1
}

type Discovery struct {
	RootTaskfileName string
}

func NewTaskDiscovery(rootTaskfileName string) *Discovery {
	return &Discovery{
		RootTaskfileName: rootTaskfileName,
	}
}

func (d *Discovery) Discover(root string, maxDepth int) (string, error) {
	includes, err := d.findComponents(root, maxDepth)
	if err != nil {
		return "", fmt.Errorf("failed to discover components: %w", err)
	}

	if len(includes) == 0 {
		return "", nil
	}

	rootTaskfilePath := filepath.Join(root, d.RootTaskfileName)

	err = d.genRootTaskfile(rootTaskfilePath, taskfileTmplData{
		Includes: includes,
	})
	if err != nil {
		return "", fmt.Errorf("failed to creat the root Taskfile: %w", err)
	}

	return rootTaskfilePath, nil
}

// findComponents looks for the {T,t}askfile.{yaml,yml} files in subdirectories (e.g. which are app framework components) of the given root directory,
// and returns discovered components
func (d *Discovery) findComponents(root string, maxDepth int) ([]componentInclude, error) {
	var includes []componentInclude

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Debug(err)
			return nil
		}

		name := strings.ToLower(d.Name())

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			log.Debug(err)
			return nil
		}

		currentDepth := depth(relPath)

		if d.IsDir() {
			if (strings.HasPrefix(name, ".") && name != ".") || currentDepth > maxDepth {
				// skip all hidden dirs (except for our root dir) or if we have already dived too deep
				return filepath.SkipDir
			}

			return nil
		}

		if name != "taskfile.yaml" && name != "taskfile.yml" {
			return nil
		}

		if currentDepth == 1 {
			// skip the root Taskfile
			return nil
		}

		dirPath := filepath.ToSlash(filepath.Dir(relPath))
		dirName := filepath.ToSlash(filepath.Base(dirPath))

		includes = append(includes, componentInclude{
			Name:     dirName,
			Taskfile: "./" + relPath,
			Dir:      "./" + dirPath,
		})

		return nil
	})

	// sort the list to make the order consistent
	sort.Slice(includes, func(i, j int) bool {
		return includes[i].Name < includes[j].Name
	})

	return includes, err
}

func (d *Discovery) genRootTaskfile(filename string, data taskfileTmplData) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	tmpl, err := tmplFS.ReadFile("Taskfile.tmpl.yaml")
	if err != nil {
		return fmt.Errorf("failed to read Taskfile template: %w", err)
	}

	var buf bytes.Buffer

	t := template.Must(template.New("taskfile").Parse(string(tmpl)))

	if err := t.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to generate Taskfile template: %w", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write Taskfile to %s: %w", filename, err)
	}

	return nil
}
