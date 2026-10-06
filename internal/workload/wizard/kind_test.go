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
	"os"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The flow tests walk every screen by keystroke and were written with the
// kind question in place. The question is kept as code and switched off by
// default (see askKind), so the package runs with it on, which keeps that
// code covered, and the tests in this file cover the default.
func TestMain(m *testing.M) {
	shippedAskKind = askKind
	askKind = true

	os.Exit(m.Run())
}

// shippedAskKind is the value model.go declares, saved before TestMain
// flips it, so one test can pin what the wizard ships.
var shippedAskKind bool

// The question is off in the binary people run. Every other test in the
// package runs with it on, so without this a flipped default stays green.
func TestKindQuestionShipsSwitchedOff(t *testing.T) {
	assert.False(t, shippedAskKind, "agents are in private preview; the kind screen ships skipped")
}

// withoutKindQuestion runs one test the way the wizard ships.
func withoutKindQuestion(t *testing.T) {
	t.Helper()

	askKind = false

	t.Cleanup(func() { askKind = true })
}

// A fresh setup goes from the name to the image source, and the manifest
// says service, which is what the skipped screen defaulted to.
func TestFlow_KindIsNotAskedWhileAgentsAreInPreview(t *testing.T) {
	withoutKindQuestion(t)

	model := newFlow(dockerfileProject(t), nil, Answers{})
	require.Equal(t, screenName, model.at)

	model = pastName(t, model)
	require.Equal(t, screenSource, model.at, "the kind question is skipped")
	assert.Equal(t, manifest.TypeService, model.draft.Type)

	model = press(t, model, "enter", "enter") // build my Dockerfile, then the settings
	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)
	assert.Contains(t, string(model.content), "type: service")

	// Going back from the source lands on the name, not on the skipped screen.
	model = press(t, model, "esc", "esc", "esc")
	assert.Equal(t, screenName, model.at)
}

// A bound workload goes straight from the binding to the image source, and an
// agent keeps its kind and its A2A answer without either screen being shown.
func TestFlow_BoundWorkloadSkipsTheKindAndKeepsIt(t *testing.T) {
	withoutKindQuestion(t)
	stubLive(t,
		documentFrom(t, `{"name": "triage-agent", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "triage-agent-artifact", "type": "agent", "spec": {"a2aEnabled": true,
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9001, "imageUri": "registry/triage:v3"}]}]}}`))

	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", Status: "running", UpdatedAt: time.Now()}}
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "enter")

	require.NoError(t, model.failed)
	require.NotNil(t, model.live)
	assert.Equal(t, screenSource, model.at)
	assert.Equal(t, manifest.TypeAgent, model.draft.Type, "the live kind is kept")
	assert.True(t, model.draft.A2AEnabled, "and so is the A2A answer")

	model = press(t, model, "esc")
	assert.Equal(t, screenBinding, model.at)
}

// A workload named by flag resumes after the name once it is fetched: at
// the image source, not at the skipped screen.
func TestFlow_FlaggedWorkloadResumesAtTheSource(t *testing.T) {
	withoutKindQuestion(t)

	model := newFlow(dockerfileProject(t), nil, Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"})
	assert.Equal(t, screenSource, model.at)
}

// The flags still take the kind headless, so an agent can be written
// without the question being asked.
func TestAnswers_TypeFlagStillWritesAnAgent(t *testing.T) {
	withoutKindQuestion(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	draft, err := Answers{Name: "my-agent", Type: manifest.TypeAgent, A2AEnabled: true}.draft(Detect(dir))
	require.NoError(t, err)
	assert.Equal(t, manifest.TypeAgent, draft.Type)
	assert.True(t, draft.A2AEnabled)
}

// With the kind screen skipped nothing could correct a bad --type, so the
// wizard refuses it before opening, the way it refuses the flag pairs no
// screen can settle.
func TestRunInteractiveFlow_RejectsABadTypeWhileTheKindScreenIsSkipped(t *testing.T) {
	withoutKindQuestion(t)

	_, _, _, err := runInteractiveFlow(
		Options{Dir: t.TempDir(), Answers: Answers{Name: "my-app", Type: "nim"}},
		dockerfileProject(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--type "nim" is not supported`)
}
