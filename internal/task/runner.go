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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

type Task struct {
	Name     string   `json:"name"`
	Desc     string   `json:"desc"`
	Summary  string   `json:"summary"`
	Aliases  []string `json:"aliases"`
	UpToDate bool     `json:"up_to_date"`
	Location struct {
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Taskfile string `json:"taskfile"`
	} `json:"location"`
}

type RunnerOpts struct {
	BinaryName string
	Dir        string
	Taskfile   string
	Stdout     *os.File
	Stderr     *os.File
	Stdin      *os.File
}

// Runner uses Taskfile to run template tasks
type Runner struct {
	opts RunnerOpts
}

func NewTaskRunner(opts RunnerOpts) *Runner {
	if opts.BinaryName == "" {
		opts.BinaryName = "task"
	}

	if opts.Dir == "" {
		opts.Dir = "."
	}

	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}

	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}

	return &Runner{
		opts: opts,
	}
}

func (r *Runner) Installed() bool {
	if _, err := exec.LookPath(r.opts.BinaryName); err != nil {
		return false
	}

	return true
}

func (r *Runner) ListTasks() ([]Task, error) {
	args := []string{"--list", "--json"}

	if r.opts.Taskfile != "" {
		args = append(args, "-t", r.opts.Taskfile)
	}

	cmd := exec.Command(r.opts.BinaryName, args...)

	cmd.Dir = r.opts.Dir

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = r.opts.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Failed to list tasks: %w", err)
	}

	var taskList struct {
		Tasks []Task `json:"tasks"`
	}

	if err := json.Unmarshal(out.Bytes(), &taskList); err != nil {
		return nil, fmt.Errorf("Failed to parse task list JSON: %w", err)
	}

	return taskList.Tasks, nil
}

type RunOpts struct {
	Parallel    bool
	WatchTask   bool
	AnswerYes   bool
	Silent      bool
	ExitCode    bool
	Concurrency int
}

func (o *RunOpts) RunArgs() []string {
	args := make([]string, 0, 6)

	if o.Parallel {
		args = append(args, "--parallel")
	}

	if o.WatchTask {
		args = append(args, "--watch")
	}

	if o.AnswerYes {
		args = append(args, "--yes")
	}

	if o.ExitCode {
		args = append(args, "--exit-code")
	}

	if o.Silent {
		args = append(args, "--silent")
	}

	args = append(args, "-C", strconv.Itoa(o.Concurrency))

	return args
}

func (r *Runner) Run(tasks []string, opts RunOpts) error {
	var args []string

	if r.opts.Taskfile != "" {
		args = append(args, "-t", r.opts.Taskfile)
	}

	args = append(args, opts.RunArgs()...)
	args = append(args, tasks...)

	cmd := exec.Command(r.opts.BinaryName, args...)

	cmd.Dir = r.opts.Dir

	cmd.Stdout = r.opts.Stdout
	cmd.Stderr = r.opts.Stderr
	cmd.Stdin = r.opts.Stdin

	return cmd.Run()
}
