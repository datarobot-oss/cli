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
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// press sends one key and returns the model it produced, so a test reads as
// the keystrokes a user would make.
func press(t *testing.T, model flow, keys ...string) flow {
	t.Helper()

	for _, key := range keys {
		updated, cmd := model.Update(keyMsg(key))

		next, ok := updated.(flow)
		require.True(t, ok, "the wizard must stay one model")

		model = next

		// Run only the commands the flow is actually waiting on. The other
		// commands a screen returns are the text input's cursor blink, a timer
		// that would make every keystroke in this test take half a second.
		if cmd != nil && model.loading != "" {
			if msg := cmd(); msg != nil {
				updated, _ = model.Update(msg)

				next, ok := updated.(flow)
				require.True(t, ok)

				model = next
			}
		}
	}

	return model
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// typeInto clears the focused field and types value into it.
func typeInto(t *testing.T, model flow, value string) flow {
	t.Helper()

	model.inputs[model.focus].SetValue("")

	for _, r := range value {
		model = press(t, model, string(r))
	}

	return model
}

// pastName answers the name screen and moves on. The screen has no default,
// so every flow that reaches a later screen has to have named the workload.
func pastName(t *testing.T, model flow) flow {
	t.Helper()

	return press(t, typeInto(t, model, "test-app"), "enter")
}

func dockerfileProject(t *testing.T) Detected {
	t.Helper()

	return Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n"))
}

// With no workloads to bind to there is no binding question, and once the
// workload is named the Dockerfile path is Enter on every remaining screen.
func TestFlow_DockerfileHappyPathIsAllEnterAfterTheName(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	require.Equal(t, screenName, model.at)

	model = pastName(t, model)
	assert.Equal(t, screenKind, model.at)

	model = press(t, model, "enter") // service
	assert.Equal(t, screenSource, model.at)

	model = press(t, model, "enter") // build my Dockerfile
	assert.Equal(t, screenSettings, model.at)

	model = press(t, model, "enter") // port and health path
	assert.Equal(t, screenConfirm, model.at)

	require.NoError(t, model.failed)
	assert.Contains(t, string(model.content), "source: provided")
	assert.Contains(t, string(model.content), "port: 3000")
}

func TestFlow_BindingScreenLeadsWhenThereAreWorkloads(t *testing.T) {
	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", Status: "running", UpdatedAt: time.Now()}}

	model := newFlow(dockerfileProject(t), workloads, Answers{})
	assert.Equal(t, screenBinding, model.at)

	// The pinned row leads, so Enter means "create a new workload".
	model = press(t, model, "enter")
	assert.Equal(t, screenName, model.at)
	assert.Nil(t, model.live)
}

// Binding downloads the live spec, skips the name question, and opens the
// remaining screens on what the workload already says.
func TestFlow_BindingDownloadsTheLiveSpec(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "triage-agent", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "triage-agent-artifact", "spec": {"type": "agent", "a2aEnabled": true,
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9001, "imageUri": "registry/triage:v3",
				 "readinessProbe": {"path": "/alive", "port": 9001}}]}]}}`))

	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", Status: "running", UpdatedAt: time.Now()}}
	model := newFlow(dockerfileProject(t), workloads, Answers{})

	model = press(t, model, "down", "enter")

	require.NoError(t, model.failed)
	require.NotNil(t, model.live)
	assert.Equal(t, screenKind, model.at, "a bound workload skips the name question")
	assert.Equal(t, manifest.TypeAgent, model.draft.Type)
	assert.Equal(t, 9001, model.draft.Port)
	assert.Equal(t, "/alive", model.draft.HealthPath)
	assert.Equal(t, manifest.BuildModeImage, model.draft.Build.Mode)
}

func TestFlow_AgentAsksAboutA2A(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})

	model = pastName(t, model)
	model = press(t, model, "2") // agent
	model = press(t, model, "enter")

	require.Equal(t, screenA2A, model.at)
	assert.False(t, model.draft.A2AEnabled, "off by default")

	model = press(t, model, "2", "enter") // yes
	assert.Equal(t, screenSource, model.at)
	assert.True(t, model.draft.A2AEnabled)
}

// Going back returns the answer that was given, not a blank screen, and
// skipped screens stay skipped in both directions.
func TestFlow_BackNavigation(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})

	model = typeInto(t, model, "chosen-name")
	model = press(t, model, "enter", "enter") // name, kind

	require.Equal(t, screenSource, model.at)

	model = press(t, model, "esc")
	assert.Equal(t, screenKind, model.at)

	model = press(t, model, "esc")
	require.Equal(t, screenName, model.at)
	assert.Equal(t, "chosen-name", model.inputs[0].Value())
}

// esc on the first screen is the same "write nothing" the confirm screen
// offers, rather than a dead end.
func TestFlow_EscOnTheFirstScreenCancels(t *testing.T) {
	model := press(t, newFlow(dockerfileProject(t), nil, Answers{}), "esc")

	assert.True(t, model.cancelled)

	_, _, err := model.result()
	require.ErrorIs(t, err, ErrCancelled)
}

// The name has no default. The placeholder shows the shape of an answer
// without proposing one, because the directory is a poor suggestion often
// enough (src, app, cli) that accepting it by reflex would put that name on a
// deployed workload.
func TestFlow_NameMustBeTyped(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	require.Equal(t, screenName, model.at)

	assert.Empty(t, model.inputs[0].Value(), "nothing to delete before typing")
	assert.Equal(t, namePlaceholder, model.inputs[0].Placeholder)
	assert.NotContains(t, model.View(), model.detected.Name,
		"the directory name is not proposed anywhere on the screen")
	assert.NotContains(t, model.View(), "artifact",
		"the derived artifact name is confirm-screen detail, not a hint here")

	// Enter on the empty field asks again rather than choosing for the user.
	empty := press(t, model, "enter")
	assert.Equal(t, screenName, empty.at)
	require.Error(t, empty.failed)
	assert.Contains(t, empty.failed.Error(), "a name is required")

	named := press(t, typeInto(t, model, "chosen"), "enter")
	require.NoError(t, named.failed)
	assert.Equal(t, screenKind, named.at)
	assert.Equal(t, "chosen", named.draft.Name)
}

// A name the user actually gave is a value, and comes back as one.
func TestFlow_TypedNameComesBackAsAValue(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})

	model = typeInto(t, model, "chosen-name")
	model = press(t, model, "enter")
	require.Equal(t, "chosen-name", model.draft.Name)

	model = press(t, model, "esc")
	require.Equal(t, screenName, model.at)
	assert.Equal(t, "chosen-name", model.inputs[0].Value(), "a given answer is shown as one")
}

// Every field on the settings screen holds its own rule, and a refused screen
// leaves the answers as the user left them rather than half-applying them.
func TestFlow_SettingsRules(t *testing.T) {
	// Index into the screen's fields: port, health, replicas, cpu, memory.
	tests := map[string]struct {
		field int
		value string
		want  string
	}{
		"privileged port":    {0, "80", "too low"},
		"port out of range":  {0, "70000", "not a port number"},
		"port not a number":  {0, "http", "must be a number"},
		"relative path":      {1, "health", "must start with /"},
		"zero replicas":      {2, "0", "1 or more"},
		"negative replicas":  {2, "-3", "1 or more"},
		"fractional replica": {2, "1.5", "whole number"},
		"zero cpu":           {3, "0", "positive number"},
		"negative cpu":       {3, "-1", "positive number"},
		"cpu not a number":   {3, "lots", "positive number"},
		"binary memory":      {4, "4Gi", "1000-based unit"},
		"nonsense memory":    {4, "big", "1000-based unit"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			model := newFlow(dockerfileProject(t), nil, Answers{})
			model = press(t, pastName(t, model), "enter", "enter")
			require.Equal(t, screenSettings, model.at)

			before := model.draft
			model.inputs[test.field].SetValue(test.value)

			model = press(t, model, "enter")

			assert.Equal(t, screenSettings, model.at)
			require.Error(t, model.failed)
			assert.Contains(t, model.failed.Error(), test.want)
			assert.Equal(t, before.Runtime, model.draft.Runtime,
				"a refused screen records nothing")
		})
	}
}

// The sizing fields reach the file, which is the point of asking for them.
func TestFlow_SettingsSizingReachesTheFile(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter")
	require.Equal(t, screenSettings, model.at)

	model.inputs[0].SetValue("9000")
	model.inputs[1].SetValue("/ready")
	model.inputs[2].SetValue("3")
	model.inputs[3].SetValue("2")
	model.inputs[4].SetValue("4GB")

	model = press(t, model, "enter")
	require.NoError(t, model.failed)
	require.Equal(t, screenConfirm, model.at)

	written := string(model.content)
	assert.Contains(t, written, "port: 9000")
	assert.Contains(t, written, "path: /ready")
	assert.Contains(t, written, "replicaCount: 3")
	assert.Contains(t, written, "cpu: 2")
	assert.Contains(t, written, "memory: 4GB")
	assert.Contains(t, model.View(), "3 replicas · 2 cpu · 4GB")
}

// Coming back shows the answers given here, not the defaults again.
func TestFlow_SettingsSurviveBackNavigation(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter")

	model.inputs[2].SetValue("7")
	model = press(t, model, "enter")
	require.Equal(t, screenConfirm, model.at)

	model = press(t, model, "esc")
	require.Equal(t, screenSettings, model.at)
	assert.Equal(t, "7", model.inputs[2].Value())
}

// Offering to build a Dockerfile that is not there would only fail later, so
// the screen says so now.
func TestFlow_DockerfileSourceNeedsADockerfile(t *testing.T) {
	model := newFlow(Detect(t.TempDir()), nil, Answers{})

	model = press(t, pastName(t, model), "enter") // name, kind
	require.Equal(t, screenSource, model.at)

	model = press(t, model, "1", "enter") // build my Dockerfile

	assert.Equal(t, screenSource, model.at)
	require.Error(t, model.failed)
	assert.Contains(t, model.failed.Error(), "no Dockerfile")
}

// With no Dockerfile the published-image option leads instead.
func TestFlow_PublishedImageLeadsWithoutADockerfile(t *testing.T) {
	model := newFlow(Detect(t.TempDir()), nil, Answers{})

	assert.Equal(t, manifest.BuildModeImage, model.draft.Build.Mode)

	model = press(t, pastName(t, model), "enter") // name, kind
	require.Equal(t, screenSource, model.at)
	assert.Equal(t, manifest.BuildModeImage, model.choice.value())

	model = press(t, model, "enter")
	require.Equal(t, screenImage, model.at)

	model = typeInto(t, model, "registry/team/app:v1")
	model = press(t, model, "enter")

	require.Equal(t, screenSettings, model.at)
	assert.Equal(t, "registry/team/app:v1", model.draft.Build.ImageURI)
}

func TestFlow_GeneratedBuildPicksABaseImage(t *testing.T) {
	original := listExecEnvsFn
	listExecEnvsFn = func(int) ([]workload.ExecutionEnvironment, error) {
		return []workload.ExecutionEnvironment{
			{ID: "68a1", Name: "[DataRobot] Python 3.12", LatestSuccessfulVersion: &workload.EEVersion{ID: "68a2"}},
		}, nil
	}

	t.Cleanup(func() { listExecEnvsFn = original })

	model := newFlow(dockerfileProject(t), nil, Answers{})

	model = press(t, pastName(t, model), "enter") // name, kind
	model = press(t, model, "2", "enter")         // build from a base image

	require.Equal(t, screenExecEnv, model.at)
	require.NoError(t, model.failed)
	assert.Empty(t, model.loading, "the picker is ready once the list arrives")

	model = press(t, model, "enter")
	require.Equal(t, screenEntrypoint, model.at)
	assert.Equal(t, "68a1", model.draft.Build.ExecutionEnvironmentID)
	assert.Equal(t, "68a2", model.draft.Build.ExecutionEnvironmentVersionID)

	model = typeInto(t, model, `uvicorn app:app --host "0.0.0.0"`)
	model = press(t, model, "enter")

	require.Equal(t, screenSettings, model.at)
	assert.Equal(t, []string{"uvicorn", "app:app", "--host", "0.0.0.0"}, model.draft.Build.Entrypoint)
}

// Confirming is the only thing that ends the flow with something to write.
func TestFlow_ConfirmProducesTheManifest(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	model = press(t, model, "enter")
	assert.True(t, model.done)

	content, draft, err := model.result()
	require.NoError(t, err)

	assert.Equal(t, "test-app", draft.Name)
	assert.Contains(t, string(content), "name: "+draft.Name)

	parsed, err := manifest.Parse(content, model.detected.Dir)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// A bound workload sees what changes, because confirming is consenting to
// change something that is running.
func TestFlow_ConfirmDiffsABoundWorkload(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "triage-agent", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "triage-agent-artifact", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9001, "imageUri": "registry/triage:v3",
				 "readinessProbe": {"path": "/alive", "port": 9001}}]}]}}`))

	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", Status: "running", UpdatedAt: time.Now()}}
	model := newFlow(dockerfileProject(t), workloads, Answers{})

	model = press(t, model, "down", "enter") // bind
	model = press(t, model, "enter")         // kind, unchanged
	model = press(t, model, "enter")         // image source, unchanged
	require.Equal(t, screenImage, model.at)

	model = press(t, model, "enter") // keep the live image
	require.Equal(t, screenSettings, model.at)

	model.inputs[0].SetValue("9100")
	model = press(t, model, "enter")

	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)

	assert.Contains(t, model.diff, "-", "the diff shows what changes")
	assert.Contains(t, model.diff, "9100")
	assert.Contains(t, model.diff, "9001")
	assert.Contains(t, model.View(), "triage-agent")
}

// An unfinished flow produces nothing to write, whichever way it ended.
func TestFlow_ResultWithoutConfirming(t *testing.T) {
	_, _, err := newFlow(dockerfileProject(t), nil, Answers{}).result()
	require.ErrorIs(t, err, ErrCancelled)
}

// Every screen renders its question, its answer and the keys that move.
func TestFlow_ViewRendersEveryScreen(t *testing.T) {
	model := newFlow(dockerfileProject(t), []workload.Workload{{ID: "68b0", Name: "triage", UpdatedAt: time.Now()}}, Answers{})

	for _, at := range []screen{
		screenBinding, screenName, screenKind, screenA2A, screenSource,
		screenEntrypoint, screenImage, screenSettings,
	} {
		model.at = at
		model.enter(at)

		view := model.View()

		assert.NotEmpty(t, model.question(), "screen %d needs a question", at)
		assert.Contains(t, view, "[esc] back", "screen %d must say how to go back", at)
	}
}
