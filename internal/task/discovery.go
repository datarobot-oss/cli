// Copyright 2025 DataRobot, Inc. and its affiliates.
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
	HasStart            bool
	StartComponents     []string
	HasLint             bool
	LintComponents      []string
	HasInstall          bool
	InstallComponents   []string
	HasUninstall        bool
	UninstallComponents []string
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
	ErrNotInTemplate     = errors.New("Not in a DataRobot template directory.")
	ErrNoTaskFilesFound  = errors.New("No Taskfiles found in child directories.")
	ErrTaskfileHasDotenv = errors.New("Existing Taskfile already has dotenv directive.")
)

// taskfileData is the structure of the .taskfile-data.yaml configuration file
// that allows template authors to provide additional data for template rendering
type taskfileData struct {
	Ports []devPort `yaml:"ports"`
}

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
	envPath := filepath.Join(root, ".datarobot")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return "", ErrNotInTemplate
	}

	includes, err := d.findComponents(root, maxDepth)
	if err != nil {
		return "", fmt.Errorf("Failed to discover components: %w", err)
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
		return "", fmt.Errorf("Failed to create the root Taskfile: %w", err)
	}

	return rootTaskfilePath, nil
}

func ExitWithError(err error) {
	if errors.Is(err, ErrNotInTemplate) {
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render("You don't seem to be in a DataRobot Template directory."))
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render("This command requires a '.datarobot' folder to be present."))
		os.Exit(1)

		return
	}

	if errors.Is(err, ErrTaskfileHasDotenv) {
		fmt.Fprintln(os.Stderr, tui.ErrorStyle.Render("Error: Cannot generate 'Taskfile' because an existing 'Taskfile' already has a dotenv directive."))
		fmt.Fprintln(os.Stderr, tui.BaseTextStyle.Render(err.Error()))
		os.Exit(1)

		return
	}

	_, _ = fmt.Fprintln(os.Stderr, "Error discovering tasks: ", err)

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
			return fmt.Errorf("Failed to read Taskfile template: %w", err)
		}
	}

	var buf bytes.Buffer

	t := template.Must(template.New("taskfile").Parse(string(tmplContent)))

	if err := t.Execute(&buf, data); err != nil {
		return fmt.Errorf("Failed to generate Taskfile template: %w", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("Failed to write Taskfile to %s: %w", filename, err)
	}

	return nil
}

func (d *Discovery) buildComposeData(root string, includes []componentInclude) (taskfileTmplData, error) {
	data := taskfileTmplData{
		Includes:            includes,
		StartComponents:     []string{},
		LintComponents:      []string{},
		InstallComponents:   []string{},
		UninstallComponents: []string{},
		TestComponents:      []string{},
		DevComponents:       []string{},
		DeployComponents:    []string{},
		DeployDevComponents: []string{},
		DevPorts:            d.loadDevPorts(root),
	}

	// Discover tasks in each component
	for _, include := range includes {
		d.aggregateComponentTasks(&data, root, include)
	}

	return data, nil
}

// aggregateComponentTasks discovers and aggregates tasks from a single component
func (d *Discovery) aggregateComponentTasks(data *taskfileTmplData, root string, include componentInclude) {
	componentPath := filepath.Join(root, include.Dir)
	runner := NewTaskRunner(RunnerOpts{
		Dir:      componentPath,
		Taskfile: filepath.Base(include.Taskfile),
	})

	tasks, err := runner.ListTasks()
	if err != nil {
		log.Debugf("Failed to list tasks for %s: %v", include.Name, err)
		return
	}

	// Check for common tasks (by name or alias)
	for _, task := range tasks {
		d.checkAndAddTask(data, task, include.Name)
	}
}

// checkAndAddTask checks if a task matches known task types and adds it to the appropriate list
func (d *Discovery) checkAndAddTask(data *taskfileTmplData, task Task, componentName string) {
	// Map task names/aliases to their component lists and flags
	taskTypeMap := map[string]struct {
		components *[]string
		hasFlag    *bool
	}{
		"start":      {&data.StartComponents, &data.HasStart},
		"lint":       {&data.LintComponents, &data.HasLint},
		"install":    {&data.InstallComponents, &data.HasInstall},
		"uninstall":  {&data.UninstallComponents, &data.HasUninstall},
		"test":       {&data.TestComponents, &data.HasTest},
		"dev":        {&data.DevComponents, &data.HasDev},
		"deploy":     {&data.DeployComponents, &data.HasDeploy},
		"up":         {&data.DeployComponents, &data.HasDeploy},
		"deploy-dev": {&data.DeployDevComponents, &data.HasDeployDev},
		"up-dev":     {&data.DeployDevComponents, &data.HasDeployDev},
	}

	// Check task name
	if target, ok := taskTypeMap[task.Name]; ok {
		addComponentOnce(target.components, target.hasFlag, componentName)
	}

	// Check aliases
	for _, alias := range task.Aliases {
		if target, ok := taskTypeMap[alias]; ok {
			addComponentOnce(target.components, target.hasFlag, componentName)
		}
	}
}

// addComponentOnce adds a component to the list if it's not already present.
// If components is nil, only sets the hasFlag (used for tasks like "start" that don't aggregate)
func addComponentOnce(components *[]string, hasFlag *bool, componentName string) {
	if components == nil {
		// Just set the flag without adding to components
		*hasFlag = true
		return
	}

	for _, c := range *components {
		if c == componentName {
			return
		}
	}

	*components = append(*components, componentName)
	*hasFlag = true
}

// loadDevPorts reads port configuration from .taskfile-data.yaml if it exists
func (d *Discovery) loadDevPorts(root string) []devPort {
	dataFile := filepath.Join(root, ".taskfile-data.yaml")

	data, err := os.ReadFile(dataFile)
	if err != nil {
		// File doesn't exist or can't be read - that's okay, it's optional
		return []devPort{}
	}

	var config taskfileData

	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Debugf("Failed to parse %s: %v", dataFile, err)
		return []devPort{}
	}

	return config.Ports
}
