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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// Prepared is a spec file read into the shape binding produces from the
// platform's two documents, so the wizard asks only what the file leaves
// open and the confirm screen and the write reuse the bound path.
type Prepared struct {
	Live manifest.Live
	// HasRuntime reports that the file sizes the workload itself, so the
	// settings screen has nothing left to ask.
	HasRuntime bool
}

// LoadSpec reads an artifact spec, the document `dr artifact create
// --spec-file` takes, or a workload spec, the one `dr workload create
// --spec-file` takes. An artifact spec becomes the artifact block of a
// workload with the default runtime; a workload spec is taken as it is. The
// file is input: the manifest is written beside the code and the file is
// left alone.
func LoadSpec(path string) (Prepared, error) {
	raw, err := workload.ReadSpecFile(path)
	if err != nil {
		return Prepared{}, err
	}

	var doc map[string]any

	if err := json.Unmarshal(raw, &doc); err != nil {
		return Prepared{}, fmt.Errorf("spec file %s is not a single object: %w", path, err)
	}

	workloadDoc, artifactDoc, err := splitSpec(doc)
	if err != nil {
		return Prepared{}, fmt.Errorf("spec file %s: %w", path, err)
	}

	live, err := manifest.NewLive("", workloadDoc, artifactDoc)
	if err != nil {
		return Prepared{}, fmt.Errorf("spec file %s has no usable artifact spec: it needs spec.containerGroups "+
			"with a primary container", path)
	}

	return Prepared{Live: live, HasRuntime: len(live.Runtime) > 0}, nil
}

// splitSpec tells the two shapes apart. A workload spec carries its artifact
// under artifact:, and one that binds an artifact by id has nothing to
// prepare from; an artifact spec is the artifact itself.
func splitSpec(doc map[string]any) (workloadDoc, artifactDoc map[string]any, err error) {
	if artifact, ok := doc["artifact"].(map[string]any); ok {
		return doc, artifact, nil
	}

	if _, ok := doc["artifactId"]; ok {
		return nil, nil, errors.New("binds an artifact by id; a prepared spec has to carry the artifact inline")
	}

	if spec, ok := doc["spec"].(map[string]any); ok && len(spec) > 0 {
		return map[string]any{}, doc, nil
	}

	return nil, nil, errors.New("is neither an artifact spec (spec.containerGroups) nor a workload spec " +
		"with an inline artifact block")
}

// specFlag is a flag a spec file already answers.
type specFlag struct {
	name string
	set  bool
}

// checkSpecFile refuses the flags a spec file already answers, and the one
// that contradicts it: a prepared spec is a workload to create.
func (o Options) checkSpecFile() error {
	if o.SpecFile == "" {
		return nil
	}

	a := o.Answers

	if a.WorkloadID != "" {
		return errors.New("--spec-file describes a workload to create and --workload-id binds an existing one; " +
			"pass one or the other")
	}

	for _, flag := range []specFlag{
		{"--build-mode", a.BuildMode != ""},
		{"--image", a.Image != ""},
		{"--execution-environment", a.ExecutionEnvironment != ""},
		{"--entrypoint", a.Entrypoint != ""},
		{"--port", a.Port != 0},
		{"--health", a.HealthPath != ""},
		{"--no-readiness-probe", a.NoProbe},
	} {
		if flag.set {
			return fmt.Errorf("%s cannot be combined with --spec-file: the file is that answer, so edit it instead", flag.name)
		}
	}

	return nil
}

// resolveHeadlessSpec is the headless run on a prepared spec: the file's
// defaults, the flags layered over them, and the bound path's render.
func (o Options) resolveHeadlessSpec(detected Detected) ([]byte, manifest.Draft, error) {
	prepared, err := LoadSpec(o.SpecFile)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	live := prepared.Live

	draft, err := o.Answers.applyTo(live.Defaults(), detected)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	if draft.Name == "" {
		return nil, manifest.Draft{}, fmt.Errorf("%s names no workload; pass --name", o.SpecFile)
	}

	if problem := preparedBuildProblem(detected, draft); problem != "" {
		return nil, manifest.Draft{}, errors.New(problem)
	}

	draft.EnvVars = live.NewEnvVars(draft.EnvVars)
	draft.EnvVars = o.storeSecrets(draft.EnvVars, detected, draft.Name, nil)

	content, err := renderPrepared(live, draft)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	warnLiveSecretLiterals(o.Stderr, live)

	return content, draft, nil
}

// renderPrepared writes the draft over the prepared spec, naming the
// artifact after the workload when the file did not.
func renderPrepared(live manifest.Live, draft manifest.Draft) ([]byte, error) {
	if live.ArtifactName == "" {
		live.ArtifactName = manifest.ArtifactName(draft.Name)
	}

	applied, err := live.Apply(draft)
	if err != nil {
		return nil, err
	}

	return applied.Render()
}

// preparedBuildProblem is why the directory cannot support the build the
// file asks for, "" when it can. A Dockerfile build is judged by the
// validation ledger, which looks for the file; a generated one is judged
// here, the way the bound path judges it.
func preparedBuildProblem(detected Detected, draft manifest.Draft) string {
	if draft.Build.Mode != manifest.BuildModeGenerated {
		return ""
	}

	if problem := detected.generatedBuild().problem; problem != "" {
		return "the spec file's generated build cannot be made from " + detected.Dir + ": " + problem
	}

	return ""
}
