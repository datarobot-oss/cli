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

package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// GeneratedTaskfileName is the Taskfile composed from the components found
// under the project root.
const GeneratedTaskfileName = "Taskfile.gen.yaml"

// RunFromRootEnv is set on a task spawned by a committed root Taskfile. That
// task calling `dr run` again must compose from components rather than resolve
// back to the file it is already running inside.
const RunFromRootEnv = "DATAROBOT_CLI_TASK_RUN_FROM_ROOT"

// RunStackEnv carries the Taskfile/task pairs of the ancestor `dr task run`
// invocations. RunFromRootEnv only breaks the one specific loop where a
// committed root Taskfile shells back into `dr run`; a project whose tasks come
// from a generated Taskfile has no such handoff, so a task there that calls
// `dr run <itself>` would otherwise fork forever.
const RunStackEnv = "DATAROBOT_CLI_TASK_STACK"

const (
	runStackEntrySep = "\n"
	runStackFieldSep = "\x1f"
)

var ErrTaskRecursion = errors.New("task recursion detected")

// componentSearchDepth is how far below the project root a component Taskfile
// may sit.
const componentSearchDepth = 2

// ResolveTaskfile returns the Taskfile a project's tasks actually come from,
// and whether that is a committed root Taskfile rather than the generated one.
//
// Every command that reads or runs a project's tasks has to agree on this.
// Resolving it differently per command means `dr task list` regenerates and
// overwrites the file `dr run` just chose, then lists tasks that are not the
// ones the next run will execute.
func ResolveTaskfile(dir string) (path string, fromRootTaskfile bool, err error) {
	if os.Getenv(RunFromRootEnv) != "" {
		path, err = NewTaskDiscovery(GeneratedTaskfileName).Discover(dir, componentSearchDepth)

		return path, false, err
	}

	discovery, err := NewDiscovery(GeneratedTaskfileName, "")
	if err != nil {
		return "", false, err
	}

	discovery.PreferRootTaskfile = true

	path, err = discovery.Discover(dir, componentSearchDepth)
	if err != nil {
		return "", false, err
	}

	return path, filepath.Base(path) != GeneratedTaskfileName, nil
}

// GuardRunStack rejects a `dr task run` that would re-enter a Taskfile/task
// pair already running above it, and returns the RunStackEnv assignment to hand
// to the task process so its own children stay guarded.
func GuardRunStack(taskfile string, tasks []string) (string, error) {
	abs, err := filepath.Abs(taskfile)
	if err != nil {
		return "", fmt.Errorf("resolving Taskfile path %q: %w", taskfile, err)
	}

	names := tasks
	if len(names) == 0 {
		// No task name means the Taskfile's default task.
		names = []string{""}
	}

	stack := os.Getenv(RunStackEnv)
	active := strings.Split(stack, runStackEntrySep)
	entries := make([]string, 0, len(names))

	for _, name := range names {
		entry := abs + runStackFieldSep + name
		if stack != "" && slices.Contains(active, entry) {
			return "", fmt.Errorf("%w: task %q in %s invokes itself; a task must not call `dr run` for a task in the same Taskfile", ErrTaskRecursion, name, abs)
		}

		entries = append(entries, entry)
	}

	if stack != "" {
		entries = append([]string{stack}, entries...)
	}

	return RunStackEnv + "=" + strings.Join(entries, runStackEntrySep), nil
}
