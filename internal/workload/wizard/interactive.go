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

package wizard

import (
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// The plumbing only the interactive flow needs. It lives apart from run.go
// so the headless command does not carry listing calls it never makes.

// Test seams for the two pickers, replaced by the wizard's tests to keep the
// flow off the network.
var (
	listWorkloadsFn = workload.ListWorkloads
	listExecEnvsFn  = workload.ListExecutionEnvironments
)

// workloadPickLimit caps the binding picker. Past this many workloads a list
// is not how anyone finds theirs, and --workload-id is the answer.
const workloadPickLimit = 100

// execEnvPickLimit caps the base-image picker for the same reason;
// --execution-environment takes a name or an id.
const execEnvPickLimit = 200

// partialDraft is draft() without the parts the flags could not settle: what
// the interactive path starts from when a flag set is incomplete or wrong.
// Every answer the flags did give survives, including the ones no screen asks
// about (the runtime sizing) and the binding, which would otherwise be lost
// and turn a bind into a create.
// draftOrPartial is the draft the flags settle, falling back to the partial
// draft when the flag set cannot be fully answered — interactively that is
// not fatal, it is what the screens exist to resolve. One name for the step
// newFlow and acceptDirectory both take, so the two cannot drift.
func (a Answers) draftOrPartial(detected Detected) manifest.Draft {
	draft, err := a.draft(detected)
	if err != nil {
		return a.partialDraft(detected)
	}

	return draft
}

func (a Answers) partialDraft(detected Detected) manifest.Draft {
	kind := manifest.ArtifactTypeOrDefault(a.Type)

	draft := manifest.Draft{
		WorkloadID: a.WorkloadID,
		Name:       orDefault(a.Name, detected.Name),
		Importance: orDefault(a.importance(), manifest.DefaultImportance),
		Type:       kind,
		A2AEnabled: a.A2AEnabled && kind == manifest.TypeAgent,
		Port:       a.Port,
		HealthPath: a.healthPath(),
		Runtime:    a.runtime(),
		EnvVars:    a.envVars(detected),
	}

	if draft.Port == 0 || checkPort(draft.Port, "--port") != nil {
		draft.Port = detected.Port
	}

	// A build the flags cannot complete is left to the source screen, which
	// opens on whatever the project suggests.
	if build, err := a.build(detected); err == nil {
		draft.Build = build
	} else {
		draft.Build = defaultBuild(detected)
	}

	return draft
}

// partialApplyTo is applyTo without the parts a screen can still settle: the
// bound counterpart of partialDraft. The flags are layered over the live spec
// so an interactive bind keeps what the command line already said, exactly as
// the headless bind does; a build the flags cannot complete leaves the live
// build alone, because the source screen is where that gets answered and it
// opens on what the workload already does.
func (a Answers) partialApplyTo(defaults manifest.Draft, detected Detected) manifest.Draft {
	draft := defaults

	if a.Name != "" {
		draft.Name = a.Name
	}

	a.applyKind(&draft)
	a.applyReadiness(&draft)
	a.applySizing(&draft)
	a.applyEnv(&draft, detected)

	// An agent flag on a service is not an answer to anything the file can
	// carry, and Apply would drop it anyway.
	draft.A2AEnabled = draft.A2AEnabled && draft.Type == manifest.TypeAgent

	if err := a.applyBuild(&draft, detected); err != nil {
		draft.Build = defaults.Build
	}

	return draft
}

// defaultBuild is the image source the project itself points at.
func defaultBuild(detected Detected) manifest.Build {
	if detected.HasDockerfile {
		return manifest.Build{Mode: manifest.BuildModeDockerfile}
	}

	return manifest.Build{Mode: manifest.BuildModeImage}
}
