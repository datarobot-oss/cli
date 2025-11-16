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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/tui"
	"gopkg.in/yaml.v3"
)

//go:embed Taskfile.tmpl.yaml
var tmplFS embed.FS

type componentInclude struct {
	Name     string
	Taskfile string
	Dir      string
}

type devPort struct {
	Name string
	Port int
}

type taskfileTmplData struct {
	Includes            []componentInclude
	HasLint             bool
	LintComponents      []string
	HasInstall          bool
	InstallComponents   []string
	HasTest             bool
	TestComponents      []string
	HasDev              bool
	DevComponents       []string
	DevPorts            []devPort
	HasDeploy           bool
	DeployComponents    []string
	HasDeployDev        bool
	DeployDevComponents []string
}

var (
	ErrNotInTemplate     = errors.New("not in a DataRobot template directory")
	ErrNoTaskFilesFound  = errors.New("no Taskfiles found in child directories")
	ErrTaskfileHasDotenv = errors.New("existing Taskfile already has dotenv directive")
)

// taskfileMetadata is used to parse just the dotenv directive from a Taskfile
type taskfileMetadata struct {
	Dotenv interface{} `yaml:"dotenv"`
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
	TemplatePath     string
}

func NewTaskDiscovery(rootTaskfileName string) *Discovery {
	return &Discovery{
		RootTaskfileName: rootTaskfileName,
	}
}

func NewComposeDiscovery(rootTaskfileName string, templatePath string) *Discovery {
	return &Discovery{
		RootTaskfileName: rootTaskfileName,
		TemplatePath:     templatePath,
	}
}

func (d *Discovery) Discover(root string, maxDepth int) (string, error) {
	// Check if .env file exists in the root directory
	envPath := filepath.Join(root, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return "", ErrNotInTemplate
	}

	includes, err := d.findComponents(root, maxDepth)
	if err != nil {
		return "", fmt.Errorf("failed to discover components: %w", err)
	}

	if len(includes) == 0 {
		return "", ErrNoTaskFilesFound
	}

	// Check if any discovered Taskfiles already have a dotenv directive
	if err := d.checkForDotenvConflicts(root, includes); err != nil {
		return "", err
	}

	rootTaskfilePath := filepath.Join(root, d.RootTaskfileName)

	composeData, err := d.buildComposeData(root, includes)
	if err != nil {
		return "", fmt.Errorf("failed to build compose data: %w", err)
	}

	err = d.genRootTaskfile(rootTaskfilePath, composeData)
	if err != nil {
		return "", fmt.Errorf("failed to create the root Taskfile: %w", err)
	}

	return rootTaskfilePath, nil
}

func ExitWithError(err error) {
	if errors.Is(err, ErrNotInTemplate) {
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render("You don't seem to be in a DataRobot Template directory."))
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render("This command requires a .env file to be present."))
		os.Exit(1)

		return
	}

	if errors.Is(err, ErrTaskfileHasDotenv) {
		fmt.Fprintln(os.Stderr, tui.ErrorStyle.Render("Error: Cannot generate Taskfile because an existing Taskfile already has a dotenv directive."))
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render(err.Error()))
		os.Exit(1)

		return
	}

	_, _ = fmt.Fprintln(os.Stderr, "Error discovering tasks:", err)

	os.Exit(1)
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

// checkForDotenvConflicts checks if any of the discovered Taskfiles already have a dotenv directive
func (d *Discovery) checkForDotenvConflicts(root string, includes []componentInclude) error {
	for _, include := range includes {
		taskfilePath := filepath.Join(root, include.Taskfile)

		hasDotenv, err := d.taskfileHasDotenv(taskfilePath)
		if err != nil {
			log.Debugf("Error checking Taskfile %s for dotenv directive: %v", taskfilePath, err)
			continue
		}

		if hasDotenv {
			return fmt.Errorf("%w: %s", ErrTaskfileHasDotenv, taskfilePath)
		}
	}

	return nil
}

// taskfileHasDotenv checks if a Taskfile has a dotenv directive
func (d *Discovery) taskfileHasDotenv(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	var meta taskfileMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return false, err
	}

	return meta.Dotenv != nil, nil
}

func (d *Discovery) genRootTaskfile(filename string, data interface{}) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	var tmplContent []byte

	// Check if custom template path is specified
	if d.TemplatePath != "" {
		tmplContent, err = os.ReadFile(d.TemplatePath)
		if err != nil {
			return fmt.Errorf("failed to read custom template: %w", err)
		}
	} else {
		// Use embedded template
		tmplContent, err = tmplFS.ReadFile("Taskfile.tmpl.yaml")
		if err != nil {
			return fmt.Errorf("failed to read Taskfile template: %w", err)
		}
	}

	var buf bytes.Buffer

	t := template.Must(template.New("taskfile").Parse(string(tmplContent)))

	if err := t.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to generate Taskfile template: %w", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write Taskfile to %s: %w", filename, err)
	}

	return nil
}

func (d *Discovery) buildComposeData(root string, includes []componentInclude) (taskfileTmplData, error) {
	data := taskfileTmplData{
		Includes:            includes,
		LintComponents:      []string{},
		InstallComponents:   []string{},
		TestComponents:      []string{},
		DevComponents:       []string{},
		DeployComponents:    []string{},
		DeployDevComponents: []string{},
		DevPorts:            []devPort{},
	}

	// Discover tasks in each component
	for _, include := range includes {
		componentPath := filepath.Join(root, include.Dir)
		runner := NewTaskRunner(RunnerOpts{
			Dir:      componentPath,
			Taskfile: filepath.Base(include.Taskfile),
		})

		tasks, err := runner.ListTasks()
		if err != nil {
			log.Debugf("Failed to list tasks for %s: %v", include.Name, err)
			continue
		}

		// Check for common tasks
		for _, task := range tasks {
			switch task.Name {
			case "lint":
				data.LintComponents = append(data.LintComponents, include.Name)
				data.HasLint = true
			case "install":
				data.InstallComponents = append(data.InstallComponents, include.Name)
				data.HasInstall = true
			case "test":
				data.TestComponents = append(data.TestComponents, include.Name)
				data.HasTest = true
			case "dev":
				data.DevComponents = append(data.DevComponents, include.Name)
				data.HasDev = true
			case "deploy":
				data.DeployComponents = append(data.DeployComponents, include.Name)
				data.HasDeploy = true
			case "deploy-dev":
				data.DeployDevComponents = append(data.DeployDevComponents, include.Name)
				data.HasDeployDev = true
			}
		}
	}

	return data, nil
}
