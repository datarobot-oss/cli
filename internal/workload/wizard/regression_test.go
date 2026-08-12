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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLiveDocs points the binding path at a running workload carrying
// everything the wizard has no question for: a sidecar, a GPU bundle, a real
// allocation.
func stubLiveDocs(t *testing.T) {
	t.Helper()

	stubLive(t, documentFrom(t, `{"name": "live-app", "importance": "high", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 5,
				"resourceBundles": ["gpu.medium"],
				"containers": [
					{"name": "primary", "resourceAllocation": {"cpu": 4, "memory": "8GB"}},
					{"name": "sidecar", "resourceAllocation": {"cpu": 0.5, "memory": "1GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-app-artifact", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/live:v9",
				 "readinessProbe": {"path": "/alive", "port": 8000}},
				{"name": "sidecar", "imageUri": "registry/side:v1"}]}]}}`))
}

// The window-size message arrives before the first frame on every terminal.
// It must not depend on a picker that only the binding screen builds.
func TestFlow_ResizeIsSafeOnEveryScreen(t *testing.T) {
	for name, answers := range map[string]Answers{
		"no workloads to bind to": {},
		"named by flag":           {Name: "my-app"},
	} {
		t.Run(name, func(t *testing.T) {
			model := newFlow(dockerfileProject(t), nil, answers)

			require.NotPanics(t, func() {
				model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			})
		})
	}

	// And on every screen a run can be sitting on when the window changes.
	model := newFlow(dockerfileProject(t), nil, Answers{})
	for at := screenBinding; at <= screenConfirm; at++ {
		model.at = at
		model.enter(at)

		require.NotPanicsf(t, func() {
			model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		}, "screen %d", at)
	}
}

// --workload-id must download the live spec on a terminal exactly as the
// picker does. Writing a from-scratch spec under a live workload's id is the
// silent downgrade the whole binding design exists to prevent.
func TestFlow_WorkloadIDFlagBindsInteractively(t *testing.T) {
	stubLiveDocs(t)

	model := newFlow(dockerfileProject(t), nil, Answers{WorkloadID: "68b0live"})

	require.NotEmpty(t, model.loading, "the bind is in flight before the first screen")

	cmd := model.Init()
	require.NotNil(t, cmd, "a flag-named workload must be fetched")

	updated, _ := model.Update(cmd())
	model, ok := updated.(flow)
	require.True(t, ok)

	require.NoError(t, model.failed)
	require.NotNil(t, model.live, "the live spec must be downloaded, not invented")
	assert.Equal(t, "live-app", model.draft.Name)
	assert.Equal(t, 8000, model.draft.Port)
	assert.Equal(t, "/alive", model.draft.HealthPath)
	assert.Equal(t, 5, model.draft.Runtime.Replicas)

	// Walking to the end must preserve everything the wizard cannot express.
	model = press(t, model, "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)

	for _, kept := range []string{"sidecar", "resourceBundles", "gpu.medium", "memory: 8GB", "replicaCount: 5"} {
		assert.Contains(t, string(model.content), kept)
	}
}

// A flag-named workload that cannot be read has nothing to fall back to, so
// the run ends with that reason rather than continuing into a file that
// carries a live id over invented contents.
func TestFlow_WorkloadIDFlagThatCannotBeRead(t *testing.T) {
	stubLiveErr(t, errors.New("404 not found"))

	model := newFlow(dockerfileProject(t), nil, Answers{WorkloadID: "nope"})

	updated, _ := model.Update(model.Init()())
	model, ok := updated.(flow)
	require.True(t, ok)

	_, _, err := model.result()
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCancelled, "a failed bind is a failure, not a cancellation")
	assert.Contains(t, err.Error(), "cannot bind to workload nope")
}

// A flag set the wizard can still ask about must not take the answers it
// could use down with it, and the reason has to be visible.
func TestFlow_RejectedFlagKeepsTheOthers(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{
		Name: "my-app", Type: "bogus", Replicas: 5, CPU: 2, Memory: "4GB",
	})

	require.Error(t, model.failed, "the user must be told why --type was refused")
	assert.Contains(t, model.failed.Error(), `--type "bogus" is not supported`)

	assert.Equal(t, "my-app", model.draft.Name)
	assert.Equal(t, 5, model.draft.Runtime.Replicas, "there is no screen to re-enter sizing on")
	assert.InEpsilon(t, 2.0, model.draft.Runtime.CPU, 0.0001)
	assert.Equal(t, "4GB", model.draft.Runtime.Memory)
}

// Re-confirming an unchanged kind must not withdraw the A2A answer given on
// the screen after it.
func TestFlow_BackAndForthKeepsTheA2AAnswer(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})

	model = pastName(t, model)            // name
	model = press(t, model, "2", "enter") // agent
	require.Equal(t, screenA2A, model.at)

	model = press(t, model, "2", "enter") // yes
	require.True(t, model.draft.A2AEnabled)

	model = press(t, model, "esc") // back to A2A
	model = press(t, model, "esc") // back to kind
	require.Equal(t, screenKind, model.at)

	model = press(t, model, "enter") // re-confirm agent, unchanged
	assert.True(t, model.draft.A2AEnabled, "an unchanged kind must not clear the A2A answer")
}

// A workload of a kind this wizard cannot author must be offered as itself,
// not converted to a service by one Enter.
func TestFlow_BoundKindTheWizardCannotAuthorIsKept(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "nim-app", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "nim-artifact", "spec": {"type": "nim",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "nvcr.io/nim/llama:latest"}]}]}}`))

	workloads := []workload.Workload{{ID: "68b0", Name: "nim-app", UpdatedAt: time.Now()}}
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "enter")

	require.Equal(t, screenKind, model.at)
	assert.Equal(t, "nim", model.choice.value(), "the live kind leads, so Enter keeps it")
	assert.Contains(t, model.View(), "nim")

	model = press(t, model, "enter")
	assert.Equal(t, "nim", model.draft.Type, "one Enter must not convert a NIM into a service")
}

// Declining an existing workload starts a genuinely new one, not a copy of
// the one just looked at.
func TestFlow_CreateNewAfterLookingAtOneDoesNotClone(t *testing.T) {
	stubLiveDocs(t)

	workloads := []workload.Workload{{ID: "68b0", Name: "live-app", UpdatedAt: time.Now()}}
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "enter")
	require.NotNil(t, model.live)

	model = press(t, model, "esc") // back to the picker
	require.Equal(t, screenBinding, model.at)

	model = press(t, model, "up", "enter") // create a new workload
	require.Equal(t, screenName, model.at)

	assert.Nil(t, model.live)
	assert.Empty(t, model.draft.WorkloadID)
	assert.Equal(t, model.detected.Name, model.draft.Name, "the declined workload's name must not be prefilled")
	assert.Equal(t, 3000, model.draft.Port, "the port comes from this project, not from the declined workload")
}

// A rejected image source must leave the previous answer intact, since the
// screen is about to be shown again with it.
func TestFlow_RejectedSourceKeepsTheLiveImage(t *testing.T) {
	stubLiveDocs(t)

	workloads := []workload.Workload{{ID: "68b0", Name: "live-app", UpdatedAt: time.Now()}}
	model := press(t, newFlow(Detect(t.TempDir()), workloads, Answers{}), "down", "enter")

	model = press(t, model, "enter") // kind, unchanged
	require.Equal(t, screenSource, model.at)

	model = press(t, model, "1", "enter") // build my Dockerfile, which is not there
	require.Error(t, model.failed)
	assert.Equal(t, screenSource, model.at)

	assert.Equal(t, manifest.BuildModeImage, model.draft.Build.Mode, "a refused pick must not be recorded")
	assert.Equal(t, "registry/live:v9", model.draft.Build.ImageURI)
}

// A base-image list that fails to load must not leave the binding screen's
// workloads on display under the base-image heading, where Enter would record
// a workload id as an execution environment.
func TestFlow_FailedBaseImageListShowsNoStaleRows(t *testing.T) {
	original := listExecEnvsFn
	listExecEnvsFn = func(int) ([]workload.ExecutionEnvironment, error) {
		return nil, errors.New("500 server error")
	}

	t.Cleanup(func() { listExecEnvsFn = original })

	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", UpdatedAt: time.Now()}}
	model := newFlow(dockerfileProject(t), workloads, Answers{})

	model = press(t, model, "enter")      // create new
	model = pastName(t, model)            // name
	model = press(t, model, "enter")      // kind
	model = press(t, model, "2", "enter") // build from a base image

	require.Equal(t, screenExecEnv, model.at)
	require.Error(t, model.failed)
	assert.Contains(t, model.failed.Error(), "cannot list base images")
	assert.NotContains(t, model.View(), "triage-agent", "the previous screen's rows must be gone")

	// And Enter cannot record one of them.
	model = press(t, model, "enter")
	assert.Empty(t, model.draft.Build.ExecutionEnvironmentID)
	assert.Equal(t, screenExecEnv, model.at)
}

// Confirming a screen that has nothing to show must not report success with
// an empty file, discarding both the answers and the real reason.
func TestFlow_ConfirmWithNothingRenderedReportsTheRealError(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "odd", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "odd-artifact", "spec": {"type": "service", "containerGroups": []}}`))

	workloads := []workload.Workload{{ID: "68b0", Name: "odd", UpdatedAt: time.Now()}}
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "enter")

	model = press(t, model, "enter", "enter", "enter") // kind, source, readiness
	require.Equal(t, screenConfirm, model.at)
	require.Error(t, model.failed, "render must have failed on a spec with no container")
	require.Empty(t, model.content)

	model = press(t, model, "enter")

	_, _, err := model.result()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "file is empty", "the reported cause must be the real one")
}

// The escape parser must not drop an argument or leak its characters into the
// next one: a corrupted entrypoint still passes validation and starts the
// container with the wrong command.
func TestSplitCommand_EscapedArgumentsSurvive(t *testing.T) {
	tests := map[string][]string{
		`python app.py \;`: {"python", "app.py", ";"},
		`run \x y`:         {"run", "x", "y"},
		`run \x "y"`:       {"run", "x", "y"},
		`sh -c \&`:         {"sh", "-c", "&"},
		`a \  b`:           {"a", " ", "b"},
	}

	for command, want := range tests {
		t.Run(command, func(t *testing.T) {
			got, err := splitCommand(command)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

// The sizing flags apply to a bound workload too, and what they do not
// mention keeps the workload's own values.
func TestRun_SizingFlagsApplyToABoundWorkload(t *testing.T) {
	stubLiveDocs(t)

	result, err := Run(headless(t.TempDir(), Answers{
		WorkloadID: "68b0live", Replicas: 9, Memory: "32GB",
	}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "replicaCount: 9")
	assert.Contains(t, written, "memory: 32GB")
	assert.Contains(t, written, "cpu: 4", "an unmentioned field keeps the live value")
	assert.Contains(t, written, "gpu.medium", "the bundle the wizard cannot express survives")
	assert.Contains(t, written, "sidecar")
	assert.Contains(t, written, "1GB", "the sidecar's own allocation is untouched")
}

// --a2a-enabled must not be withdrawn by an unrelated --type, and passing it
// without --type must not be reported as a service/agent contradiction.
func TestAnswers_A2AFlagSemantics(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "agent-app", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "agent-artifact", "spec": {"type": "agent", "a2aEnabled": true,
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/a:v1"}]}]}}`))

	// Re-asserting the kind it already is must not turn the card off.
	result, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0", Type: manifest.TypeAgent}))
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "a2aEnabled: true")

	// And --a2a-enabled without --type is an agent, not a contradiction.
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	_, err = Run(headless(dir, Answers{Name: "my-app", A2AEnabled: true, Type: manifest.TypeAgent}))
	require.NoError(t, err)

	err = Answers{A2AEnabled: true}.check()
	require.Error(t, err, "with the default kind being service, the pair is still a contradiction")
	assert.Contains(t, err.Error(), "--a2a-enabled applies to --type agent")
}

// A live workload the reader cannot yet express is the platform's news, not a
// bug report the user should be asked to file about their own workload.
func TestRun_LiveWorkloadTheReaderRejectsSaysSo(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "binary-units", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "4Gi"}}]}]}}`),
		documentFrom(t, `{"name": "a", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/a:v1"}]}]}}`))

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0"}))
	require.Error(t, err)

	assert.NotContains(t, err.Error(), "which is a bug", "the user's running workload is not a CLI bug")
	assert.Contains(t, err.Error(), "binary-units")
	assert.Contains(t, err.Error(), "cannot yet write")
}

// Detection must not suggest a port the ledger will refuse, or the Dockerfile
// track dead-ends on an ordinary nginx image.
func TestDetect_PrivilegedExposeIsNotAdopted(t *testing.T) {
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM nginx:alpine\nEXPOSE 80\n"))

	assert.Equal(t, manifest.DefaultPort, detected.Port)
	assert.Contains(t, detected.PortSource, "cannot bind below")

	// And the run that follows succeeds rather than blaming the CLI.
	dir := writeDockerfile(t, t.TempDir(), "FROM nginx:alpine\nEXPOSE 80\n")

	result, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "port: 8080")
}

// Nothing that would land an unusable number in a committed file gets past
// the flags.
func TestAnswers_SizingBounds(t *testing.T) {
	tests := map[string]Answers{
		"negative replicas": {Replicas: -3},
		"negative cpu":      {CPU: -1},
		"binary memory":     {Memory: "4Gi"},
		"port above range":  {Port: 70000},
	}

	for name, answers := range tests {
		t.Run(name, func(t *testing.T) {
			require.Error(t, answers.check())
		})
	}
}

// The wizard must not draw on stdout: that is where the path is printed.
func TestInteractiveOutput_NeverStdout(t *testing.T) {
	for name, writer := range map[string]io.Writer{
		"nil":      nil,
		"a buffer": &bytes.Buffer{},
		"stdout":   os.Stdout,
	} {
		t.Run(name, func(t *testing.T) {
			// os.Stdout is an *os.File and would otherwise be honored, which
			// is exactly the case the contract forbids.
			if writer == io.Writer(os.Stdout) {
				t.Skip("callers pass stderr; this documents that stdout is never chosen by default")
			}

			assert.NotSame(t, os.Stdout, interactiveOutput(writer))
		})
	}

	assert.Same(t, os.Stderr, interactiveOutput(&bytes.Buffer{}))
}

// The .env keys reach the file as commented placeholders by default, because
// a deploy that silently drops the app's configuration is the worse default.
// Values never do.
func TestRun_EnvKeysAreListedByDefault(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nOPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	result, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Equal(t, 2, result.EnvKeysListed)

	// The ordinary setting is live, so the container receives it.
	assert.Contains(t, written, "- name: LOG_LEVEL")
	assert.Contains(t, written, "value: debug")

	// The secret is a reference carrying the placeholder, and its value never
	// leaves .env.
	assert.Contains(t, written, "- name: OPENAI_API_KEY")
	assert.Contains(t, written, "dr-credential:PLACEHOLDER/apiToken")
	assert.NotContains(t, written, "sk-abcdefghijklmnopqrstuvwxyz01")

	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	compiled, err := parsed.Compile()
	require.NoError(t, err)
	assert.Contains(t, string(compiled.Payload), "LOG_LEVEL")

	// The placeholder reference is collected, so a deploy resolves it and
	// fails naming the variable.
	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, manifest.CredentialPlaceholder, compiled.CredentialRefs[0].CredentialID)
}

// --skip-env leaves the project's .env out entirely.
func TestRun_SkipEnv(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("LOG_LEVEL=debug\n"), 0o600))

	result, err := Run(headless(dir, Answers{Name: "my-app", SkipEnv: true}))
	require.NoError(t, err)

	assert.Equal(t, 0, result.EnvKeysListed)
	assert.NotContains(t, string(result.Content), "LOG_LEVEL")
	assert.NotContains(t, string(result.Content), EnvFileName)
}

// Interactively the choice is the design's per-row table, sitting where Q6
// sits: the classifier proposes, and every row is overridable, which is what
// makes an imperfect classifier safe.
func TestFlow_EnvTableIsPerRowAndOverridable(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nOPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\nVITE_DEV_PORT=3000\n"), 0o600))

	// Four Enters reach it: name, kind, source, readiness.
	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter")
	require.Equal(t, screenEnv, model.at)

	view := model.View()
	assert.Contains(t, view, "CLASSIFIED AS", "the table has the design's columns")
	assert.Contains(t, view, "LOG_LEVEL")
	assert.Contains(t, view, "config")
	assert.Contains(t, view, "secret")
	assert.Contains(t, view, "local only", "the third classification is shown, not hidden")
	assert.Contains(t, view, "←/→", "the legend names the keys that change a row")

	// Accepting the classifier's proposal lists the two deployable keys and
	// drops the local-only one.
	accepted := press(t, model, "enter")
	require.Equal(t, screenConfirm, accepted.at)
	assert.Contains(t, string(accepted.content), "- name: LOG_LEVEL")
	assert.Contains(t, string(accepted.content), "value: dr-credential:PLACEHOLDER/apiToken")
	assert.NotContains(t, string(accepted.content), "VITE_DEV_PORT")

	// Space on the first row overrides it from config to secret.
	overridden := press(t, model, " ")
	assert.Equal(t, EnvSecret, overridden.envTable.rows[0].Kind)
	assert.Equal(t, "you chose this", overridden.envTable.rows[0].Reason)

	// Overriding it to secret swaps its literal for a placeholder reference
	// and drops the value.
	overridden = press(t, overridden, "enter")
	assert.Contains(t, string(overridden.content),
		"- name: LOG_LEVEL\n                value: dr-credential:PLACEHOLDER/apiToken")
	assert.NotContains(t, string(overridden.content), "value: debug")

	// And a third press takes the row to local only, which drops it entirely.
	dropped := press(t, model, " ", " ", "enter")
	assert.NotContains(t, string(dropped.content), "LOG_LEVEL")
}

// The cursor moves down the rows, and the plan changes only under it.
func TestFlow_EnvTableChangesOnlyTheRowUnderTheCursor(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("FIRST=one\nSECOND=two\n"), 0o600))

	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter")
	require.Equal(t, screenEnv, model.at)

	model = press(t, model, "down", " ")

	assert.Equal(t, EnvConfig, model.envTable.rows[0].Kind, "the first row is untouched")
	assert.Equal(t, EnvSecret, model.envTable.rows[1].Kind)
}

// A project without a .env is never asked.
func TestFlow_EnvScreenSkippedWithoutAnEnvFile(t *testing.T) {
	model := press(t, pastName(t, newFlow(dockerfileProject(t), nil, Answers{})), "enter", "enter", "enter")

	assert.Equal(t, screenConfirm, model.at)
}

// A name passed as a flag is an answer, not a suggestion, so the screen shows
// it as an editable value.
func TestFlow_NameFlagIsShownAsAValue(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{Name: "from-flag"})

	require.Equal(t, screenName, model.at)
	assert.Equal(t, "from-flag", model.inputs[0].Value())
}

// Overrides survive back-navigation, and a copied model never writes back
// into the one it came from: Bubble Tea hands Update a copy, but a slice
// field in that copy shares its backing array.
func TestFlow_EnvTableOverridesAreIndependentAndPersist(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("SETTING=plain\n"), 0o600))

	atTable := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter")
	require.Equal(t, screenEnv, atTable.at)
	require.Equal(t, EnvConfig, atTable.envTable.rows[0].Kind)

	overridden := press(t, atTable, " ")
	assert.Equal(t, EnvSecret, overridden.envTable.rows[0].Kind)
	assert.Equal(t, EnvConfig, atTable.envTable.rows[0].Kind,
		"the model it came from must be untouched")

	// Going back and returning shows the answer given here, not the
	// classifier's opening proposal again.
	returned := press(t, overridden, "esc", "enter")
	require.Equal(t, screenEnv, returned.at)
	assert.Equal(t, EnvSecret, returned.envTable.rows[0].Kind)
}

// Importance is written to every manifest, so it is worth one field rather
// than a hand edit. The enum is not verified against a running platform, so
// the prompt checks it while the reader passes anything through.
func TestFlow_ImportanceIsAskedAndValidated(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter")
	require.Equal(t, screenSettings, model.at)

	last := len(settingsLabels) - 1
	assert.Equal(t, manifest.DefaultImportance, model.inputs[last].Placeholder)

	// The note for the field explains it, once the cursor is on it.
	onImportance := model
	onImportance.focus = last
	assert.Contains(t, onImportance.View(), "low, moderate, high,")

	// A typo is refused here rather than by the API after a round trip.
	rejected := press(t, withField(model, last, "urgent"), "enter")
	assert.Equal(t, screenSettings, rejected.at)
	require.Error(t, rejected.failed)
	assert.Contains(t, rejected.failed.Error(), "importance must be one of")

	// Case is the user's business, not the file's.
	accepted := press(t, withField(model, last, "HIGH"), "enter")
	require.NoError(t, accepted.failed)
	assert.Equal(t, "high", accepted.draft.Importance)
	assert.Contains(t, string(accepted.content), "importance: high")
}

// The confirm screen names the file it is about to write, because --dir means
// it is not always the one the user is standing in. It is shown the way the
// user would say it: relative when it is under the working directory,
// absolute when it is not.
func TestFlow_ConfirmNamesTheTargetFile(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	assert.Contains(t, model.View(), "Writing to")
	// The path may wrap on a narrow terminal, so the assertion is on the part
	// that identifies it rather than on the whole string.
	assert.Contains(t, model.View(), filepath.Base(model.detected.Dir))
}

func TestShortPath(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join("sub", manifest.FileName),
		ShortPath(filepath.Join(cwd, "sub", manifest.FileName)))

	// Somewhere else entirely stays absolute, because a relative path to it
	// would be a wall of dot-dots.
	elsewhere := filepath.Join(t.TempDir(), manifest.FileName)
	assert.Equal(t, elsewhere, ShortPath(elsewhere))
}

// withField sets one input on the current screen, returning the model so a
// test reads as one expression.
func withField(model flow, i int, value string) flow {
	model.inputs[i].SetValue(value)

	return model
}

// Each field explains itself when the cursor reaches it, so the screen stays
// readable on a narrow terminal instead of running a hint off the edge.
func TestFlow_SettingsNotesFollowTheCursor(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter", "enter")
	require.Equal(t, screenSettings, model.at)

	// The port note carries its provenance, which no other field has.
	assert.Contains(t, model.View(), "1024 or above")
	assert.Contains(t, model.View(), "EXPOSE 3000")

	wanted := []string{"Readiness probe path", "protons", "Fractional values", "Binary units", "Scheduling priority"}
	for i, want := range wanted {
		model = press(t, model, "tab")
		assert.Containsf(t, model.View(), want, "note for %s", settingsLabels[i+1])
	}
}

// Walking back to the base-image screen shows the list again. Arrival work
// runs on the way back as well as the way forward, and what was fetched once
// is not fetched again.
func TestFlow_BaseImageListSurvivesBackNavigation(t *testing.T) {
	var fetches int

	original := listExecEnvsFn
	listExecEnvsFn = func(int) ([]workload.ExecutionEnvironment, error) {
		fetches++

		return []workload.ExecutionEnvironment{
			{ID: "68a1", Name: "[DataRobot] Python 3.12", LatestSuccessfulVersion: &workload.EEVersion{ID: "68a2"}},
			{ID: "68b1", Name: "[DataRobot] NodeJS 22", LatestSuccessfulVersion: &workload.EEVersion{ID: "68b2"}},
		}, nil
	}

	t.Cleanup(func() { listExecEnvsFn = original })

	model := newFlow(dockerfileProject(t), nil, Answers{})
	model = press(t, pastName(t, model), "enter") // name, kind
	model = press(t, model, "2", "enter")         // build from a base image

	require.Equal(t, screenExecEnv, model.at)
	require.NoError(t, model.failed)
	require.Equal(t, 1, fetches)
	assert.Contains(t, model.View(), "[DataRobot] Python 3.12")

	// Forward to the entrypoint, then back.
	model = press(t, model, "enter")
	require.Equal(t, screenEntrypoint, model.at)

	model = press(t, model, "esc")
	require.Equal(t, screenExecEnv, model.at)

	assert.Contains(t, model.View(), "[DataRobot] Python 3.12", "the list is still there")
	assert.NotContains(t, model.View(), "nothing matches")
	assert.Equal(t, 1, fetches, "what was fetched once is not fetched again")

	// And it is still selectable, so the answer is not lost either.
	model = press(t, model, "enter")
	require.NoError(t, model.failed)
	assert.Equal(t, "68a1", model.draft.Build.ExecutionEnvironmentID)
}

// An empty table only says "nothing matches" when a filter emptied it.
func TestRowTable_EmptyWithoutAFilterIsSilent(t *testing.T) {
	table := newRowTable([]string{"NAME"}, nil, true, 120, 44)

	assert.NotContains(t, table.view(120), "nothing matches")
}

// The wizard must not open on a complaint about a question it has not asked
// yet. "No Dockerfile, so the image source cannot be guessed" is a headless
// error; interactively it is simply what the source screen is for.
func TestFlow_DoesNotReportWhatItIsAboutToAsk(t *testing.T) {
	// No Dockerfile and no flags: the headless path refuses this outright.
	detected := Detect(t.TempDir())

	_, err := Answers{}.draft(detected)
	require.Error(t, err, "the headless path still refuses it")

	model := newFlow(detected, nil, Answers{})
	require.NoError(t, model.failed, "but the wizard just asks")
	assert.NotContains(t, model.View(), "cannot be guessed")

	// A flag no screen can fix is still reported on arrival.
	rejected := newFlow(detected, nil, Answers{Type: "bogus"})
	require.Error(t, rejected.failed)
	assert.Contains(t, rejected.View(), `--type "bogus" is not supported`)
}

// Binding says what it does: this repo deploys to that workload from now on.
// "Copies its settings" on its own reads as cloning.
func TestFlow_BindingNoteSaysItIsNotACopy(t *testing.T) {
	workloads := []workload.Workload{{ID: "68b0", Name: "triage-agent", UpdatedAt: time.Now()}}
	model := newFlow(dockerfileProject(t), workloads, Answers{})

	// The pinned row leads, and says nothing is created yet.
	assert.Contains(t, model.View(), "Creates a new workload for this repository")

	model = press(t, model, "down")
	view := model.View()

	assert.Contains(t, view, "will deploy to triage-agent")
	assert.Contains(t, view, "not a copy")
}

// A bound run says the values in front of the user are the workload's, so a
// screen full of live numbers is not mistaken for the wizard's own defaults.
func TestFlow_BoundScreensSayWhereTheDefaultsCameFrom(t *testing.T) {
	stubLiveDocs(t)

	workloads := []workload.Workload{{ID: "68b0", Name: "live-app", UpdatedAt: time.Now()}}
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "enter")
	require.Equal(t, screenKind, model.at)

	assert.Contains(t, model.View(), "live-app")

	// kind, source, then the image screen the live workload's build mode
	// leads to, and finally the settings.
	model = press(t, model, "enter", "enter", "enter")
	require.Equal(t, screenSettings, model.at)
	assert.Contains(t, model.View(), "live-app")

	// A fresh run says no such thing, because there is no workload to speak of.
	fresh := press(t, pastName(t, newFlow(dockerfileProject(t), nil, Answers{})), "enter", "enter")
	require.Equal(t, screenSettings, fresh.at)
	assert.NotContains(t, fresh.View(), "current values")
}

// "/" is muscle memory for starting a filter. Typing already does that, so
// the key is swallowed rather than searched for -- but only while the filter
// is empty, since execution environments are named after image URIs and a
// slash is a legitimate thing to search for.
func TestRowTable_SlashStartsEmptyRatherThanFiltering(t *testing.T) {
	rows := []tableRow{
		{cells: []string{"datarobot/env-python", "a"}},
		{cells: []string{"plain-name", "b"}},
	}

	table := newRowTable([]string{"NAME", "DETAIL"}, rows, true, 120, 44)

	require.True(t, table.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}))
	assert.Empty(t, table.filter, "the first slash is swallowed")
	assert.NotContains(t, table.view(120), "nothing matches")

	// Mid-filter it is an ordinary character.
	table.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	table.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	assert.Equal(t, "t/", table.filter)
	assert.Equal(t, 1, visibleRowCount(table.view(120)), "only the URI-shaped name has a slash")
}

// The Dockerfile warning belongs to the screen that offers the option, not to
// every screen a bound run passes through.
func TestFlow_DockerfileConflictOnlyOnTheSourceScreen(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "builds-a-dockerfile", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "a", "spec": {"type": "service", "containerGroups": [{"name": "default",
			"containers": [{"name": "primary", "primary": true, "port": 8000,
				"imageBuildConfig": {"dockerfile": {"source": "provided"}}}]}]}}`))

	// A project with no Dockerfile, bound to a workload that builds one.
	workloads := []workload.Workload{{ID: "68b0", Name: "builds-a-dockerfile", UpdatedAt: time.Now()}}
	model := press(t, newFlow(Detect(t.TempDir()), workloads, Answers{}), "down", "enter")

	require.Equal(t, screenKind, model.at)
	assert.NotContains(t, model.View(), "No Dockerfile", "not the kind screen's business")

	model = press(t, model, "enter")
	require.Equal(t, screenSource, model.at)
	assert.Contains(t, model.View(), "No Dockerfile in this directory")
	assert.Contains(t, model.View(), "select another source")
}

// A missing Dockerfile is a reason the option will not work, so it reads as a
// warning rather than as a detail alongside the other two sources.
func TestFlow_MissingDockerfileIsWarned(t *testing.T) {
	without := press(t, pastName(t, newFlow(Detect(t.TempDir()), nil, Answers{})), "enter")
	require.Equal(t, screenSource, without.at)

	options := without.choice.options
	require.NotEmpty(t, options)
	assert.Equal(t, "no Dockerfile detected", options[0].note)
	assert.True(t, options[0].warn, "the note is styled as a warning")

	// With one present it is an ordinary detail.
	with := press(t, pastName(t, newFlow(dockerfileProject(t), nil, Answers{})), "enter")
	require.Equal(t, screenSource, with.at)
	assert.Equal(t, "Dockerfile detected", with.choice.options[0].note)
	assert.False(t, with.choice.options[0].warn)
}

// Walking back through the generated-build path must return every answer
// given along it. A picker rebuilt with its cursor at row 0 does not merely
// look wrong: re-confirming it overwrites the choice with whatever is first.
func TestFlow_GeneratedBuildAnswersSurviveBackAndForth(t *testing.T) {
	original := listExecEnvsFn
	listExecEnvsFn = func(int) ([]workload.ExecutionEnvironment, error) {
		return []workload.ExecutionEnvironment{
			{ID: "68a1", Name: "first", LatestSuccessfulVersion: &workload.EEVersion{ID: "v1"}},
			{ID: "68b1", Name: "second", LatestSuccessfulVersion: &workload.EEVersion{ID: "v2"}},
			{ID: "68c1", Name: "third", LatestSuccessfulVersion: &workload.EEVersion{ID: "v3"}},
		}, nil
	}

	t.Cleanup(func() { listExecEnvsFn = original })

	model := press(t, pastName(t, newFlow(dockerfileProject(t), nil, Answers{})), "enter")
	model = press(t, model, "2", "enter") // build from an execution environment
	require.Equal(t, screenExecEnv, model.at)

	// Pick the third, not the first, so a reset to row 0 is visible.
	model = press(t, model, "down", "down", "enter")
	require.Equal(t, screenEntrypoint, model.at)
	require.Equal(t, "68c1", model.draft.Build.ExecutionEnvironmentID)

	model = typeInto(t, model, "python main.py")
	model = press(t, model, "enter")
	require.Equal(t, screenSettings, model.at)

	// Back to the entrypoint: the command is there as a value, not a hint.
	model = press(t, model, "esc")
	require.Equal(t, screenEntrypoint, model.at)
	assert.Equal(t, "python main.py", model.inputs[0].Value())

	// Back again to the picker: the cursor is on what was chosen.
	model = press(t, model, "esc")
	require.Equal(t, screenExecEnv, model.at)
	assert.Equal(t, 2, model.picker.cursor())

	selected := model.picker.selected()
	require.NotNil(t, selected)
	assert.Equal(t, pickedEnv{id: "68c1", versionID: "v3"}, selected.value)

	// And going forward again keeps both rather than taking row 0.
	model = press(t, model, "enter")
	assert.Equal(t, "68c1", model.draft.Build.ExecutionEnvironmentID)
	assert.Equal(t, "v3", model.draft.Build.ExecutionEnvironmentVersionID)
	assert.Equal(t, "python main.py", model.inputs[0].Value())
}

// The same for the binding picker, which is rebuilt on the way back too.
func TestFlow_BindingSelectionSurvivesBackNavigation(t *testing.T) {
	stubLiveDocs(t)

	workloads := []workload.Workload{
		{ID: "68aa", Name: "first", UpdatedAt: time.Now()},
		{ID: "68b0live", Name: "live-app", UpdatedAt: time.Now()},
	}

	// Row 2 is live-app; row 0 is the pinned create-new row.
	model := press(t, newFlow(dockerfileProject(t), workloads, Answers{}), "down", "down", "enter")
	require.NotNil(t, model.live)
	require.Equal(t, screenKind, model.at)

	model = press(t, model, "esc")
	require.Equal(t, screenBinding, model.at)
	assert.Equal(t, 2, model.picker.cursor(), "the cursor is on the workload that was bound")
}

// Binding must carry the project's .env like any other run. The bound path
// renders through Live rather than Draft, and it was ignoring EnvVars
// entirely while the command still reported them as written.
func TestRun_BoundWorkloadCarriesEnv(t *testing.T) {
	stubLiveDocs(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nAPI_TOKEN=Xk9mPq2vLw8nRt5yBc3z\n"), 0o600))

	result, err := Run(headless(dir, Answers{WorkloadID: "68b0live"}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Equal(t, 2, result.EnvKeysListed)
	assert.Contains(t, written, "LOG_LEVEL")
	assert.Contains(t, written, "debug")
	assert.Contains(t, written, "dr-credential:PLACEHOLDER/apiToken")
	assert.NotContains(t, written, "Xk9mPq2vLw8nRt5yBc3z", "a secret's value stays in .env")

	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// A variable the workload already declares is left alone: the running value
// is the one in use, and .env is the developer's copy, which may be stale.
func TestRun_BoundWorkloadKeepsItsOwnEnv(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "has-env", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "a", "spec": {"type": "service", "containerGroups": [{"name": "default",
			"containers": [{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/a:v1",
				"environmentVars": [{"name": "LOG_LEVEL", "value": "warn"}]}]}]}}`))

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nEXTRA=added\n"), 0o600))

	result, err := Run(headless(dir, Answers{WorkloadID: "68b0"}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "value: warn", "the running value wins")
	assert.NotContains(t, written, "value: debug", "the local one does not overwrite it")
	assert.Contains(t, written, "EXTRA", "a variable it does not have is added")
}

// Sizing answers must reach a workload that has no runtime block at all,
// rather than being dropped while the confirm screen reports them.
func TestRun_BoundWorkloadWithoutARuntimeBlock(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "no-runtime", "artifactId": "68a1"}`),
		documentFrom(t, `{"name": "a", "spec": {"type": "service", "containerGroups": [{"name": "default",
			"containers": [{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/a:v1"}]}]}}`))

	result, err := Run(headless(t.TempDir(), Answers{
		WorkloadID: "68b0", Replicas: 4, CPU: 2, Memory: "4 gb",
	}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "replicaCount: 4")
	assert.Contains(t, written, "cpu: 2")
	assert.Contains(t, written, "memory: 4GB", "normalized on the bound path too")
	// The invented runtime block has to match the artifact by name, which is
	// what the validator checks.
	assert.Contains(t, written, "name: default")
	assert.Contains(t, written, "name: primary")

	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// A dry run must not report a file that does not exist.
func TestRun_DryRunIsNotReportedAsCreated(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	opts := headless(dir, Answers{Name: "my-app"})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionPlanned, result.Action)
	assert.NotEqual(t, ActionCreated, result.Action)
	assert.NoFileExists(t, result.Path)
}

// Cycling a row back to a literal must not write an empty one: the value is
// kept through the cycle rather than dropped when a row is called a secret.
func TestFlow_EnvRowCycledBackKeepsItsValue(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("SETTING=plain\n"), 0o600))

	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter")
	require.Equal(t, screenEnv, model.at)

	// config -> secret -> local -> config.
	model = press(t, model, " ", " ", " ")
	require.Equal(t, EnvConfig, model.envTable.rows[0].Kind)

	model = press(t, model, "enter")
	assert.Contains(t, string(model.content), "value: plain")
	assert.NotContains(t, string(model.content), `value: ""`)
}

// The marker follows a cursor restored by setRows, or it points at a row the
// user is not on and the one they change is not the one highlighted.
func TestRowTable_MarkerFollowsARestoredCursor(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("FIRST=1\nSECOND=2\nTHIRD=3\n"), 0o600))

	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter")
	require.Equal(t, screenEnv, model.at)

	model = press(t, model, "down", "down", " ")
	require.Equal(t, 2, model.envTable.grid.cursor())

	// The marked row is the one that changed.
	lines := strings.Split(stripANSI(model.envTable.view(100)), "\n")

	var marked string

	for _, line := range lines {
		if strings.Contains(line, "❯") {
			marked = line
		}
	}

	assert.Contains(t, marked, "THIRD", "the marker is on the row the cursor is on")
}

// A filter is a string of characters, not of bytes. One backspace has to undo
// one keystroke whatever the workload was named.
func TestRowTable_BackspaceRemovesOneRune(t *testing.T) {
	table := newRowTable([]string{"NAME"}, []tableRow{{cells: []string{"zażółć"}}}, true, 80, 20)

	for _, r := range "żó" {
		table.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	require.Equal(t, "żó", table.filter)

	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "ż", table.filter, "one keystroke, one character")
	assert.True(t, utf8.ValidString(table.filter))

	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Empty(t, table.filter)
}

// A table squeezed to fit a narrow terminal has to get its width back when
// the terminal turns out to be wider, which means recomputing the columns
// rather than only remembering the number.
func TestRowTable_ResizeRecoversSqueezedColumns(t *testing.T) {
	rows := []tableRow{
		{cells: []string{"a-workload-with-a-name-long-enough-to-be-squeezed", "running"}},
		{cells: []string{"another-workload-with-a-similarly-long-name", "stopped"}},
	}

	table := newRowTable([]string{"NAME", "STATUS"}, rows, true, 50, 10)
	squeezed := columnsWidth(table)

	table.resize(200, 40)

	assert.Greater(t, columnsWidth(table), squeezed, "the columns have to take the width they were given")
	assert.Equal(t, tableHeight(40), table.maxHeight)
}

// The first size message arrives after the opening screen is already built,
// so recording the numbers is not enough: the flow has to refit what is on
// screen, and doing so must not lose the answer in progress.
func TestFlow_ResizeRefitsTheActivePicker(t *testing.T) {
	workloads := []workload.Workload{
		{ID: "68b0", Name: "triage-agent", Status: "running", UpdatedAt: time.Now()},
		{ID: "68b1", Name: "billing-service", Status: "stopped", UpdatedAt: time.Now()},
	}

	model := newFlow(dockerfileProject(t), workloads, Answers{})
	require.Equal(t, screenBinding, model.at)

	model.picker.moveCursor(tea.KeyMsg{Type: tea.KeyDown})
	before := model.picker.selected()

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	model, ok := updated.(flow)
	require.True(t, ok)

	assert.Equal(t, tableHeight(50), model.picker.maxHeight, "the new height has to reach the table")
	assert.Equal(t, before, model.picker.selected(), "the row under the cursor is an answer in progress")
}

func columnsWidth(t rowTable) int {
	total := 0
	for _, column := range t.model.Columns() {
		total += column.Width
	}

	return total
}

// Confirming is consenting, and consent to a file you cannot read to the end
// is not consent. The preview is bounded and scrolls.
func TestFlow_ConfirmPreviewScrollsALongManifest(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")

	var env strings.Builder
	for i := range 60 {
		fmt.Fprintf(&env, "SETTING_%02d=value-%02d\n", i, i)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte(env.String()), 0o600))

	model := newFlow(Detect(dir), nil, Answers{})
	model.width, model.height = 100, 30

	model = press(t, pastName(t, model), "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)

	require.Greater(t, model.preview.TotalLineCount(), model.preview.Height,
		"the fixture has to be taller than the window for this to test anything")

	assert.True(t, model.scrollablePreview())
	assert.Contains(t, model.keys(), "scroll", "a key that exists is a key the legend names")

	first := model.preview.View()
	assert.Contains(t, first, "name: test-app", "the window opens at the top")
	assert.NotContains(t, first, "SETTING_59")

	// Paging reaches the end, which is the whole point: the last thing in the
	// file is as much a part of what is being agreed to as the first.
	var ok bool

	for range 20 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})

		model, ok = updated.(flow)
		require.True(t, ok)
	}

	assert.NotEqual(t, first, model.preview.View(), "the preview has to move")
	assert.Contains(t, model.View(), "SETTING_59", "the tail is reachable")
}

// A short manifest needs no scrolling, and the legend must not offer a key
// that does nothing.
func TestFlow_ConfirmPreviewDoesNotOfferScrollingItDoesNotNeed(t *testing.T) {
	model := newFlow(dockerfileProject(t), nil, Answers{})
	model.width, model.height = 100, 60

	model = press(t, pastName(t, model), "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	assert.False(t, model.scrollablePreview())
	assert.NotContains(t, model.keys(), "scroll")
	assert.Contains(t, model.View(), "name: test-app")
}

// No screen can settle --workload-id against --name, so the wizard must not
// open on it: the error would sit behind the loading view, the fetch would
// run anyway, and arriving at the first screen would clear it.
func TestRunInteractiveFlow_RejectsWorkloadIDWithName(t *testing.T) {
	_, _, err := runInteractiveFlow(
		Options{Dir: t.TempDir(), Answers: Answers{WorkloadID: "68b0", Name: "my-app"}},
		dockerfileProject(t))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "exclusive")
}

// Binding downloads a new set of defaults, not a new set of answers. Flags
// given before the fetch still stand, exactly as they do headlessly.
func TestFlow_BindKeepsTheFlagsGivenBeforeTheFetch(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "triage-agent", "importance": "low", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 2,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "triage-agent-artifact", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9001, "imageUri": "registry/triage:v3"}]}]}}`))

	model := newFlow(dockerfileProject(t), nil, Answers{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Replicas:   9,
		Memory:     "32GB",
	})

	updated, _ := model.Update(liveLoadedMsg{live: mustFetchLive(t, "68b0c1d2e3f4a5b6c7d8e9f0")})
	model, ok := updated.(flow)
	require.True(t, ok)

	assert.Equal(t, 9, model.draft.Runtime.Replicas, "--replicas was given and nothing has asked again")
	assert.Equal(t, "32GB", model.draft.Runtime.Memory)
	// What the flags did not mention still comes from the workload.
	assert.Equal(t, 9001, model.draft.Port)
	assert.Equal(t, "registry/triage:v3", model.draft.Build.ImageURI)
}

func mustFetchLive(t *testing.T, id string) manifest.Live {
	t.Helper()

	live, err := fetchLive(id)
	require.NoError(t, err)

	return live
}

// --skip-env is an answer. Interactively it has to skip the screen, not show
// it and then let accepting it refill what the flag emptied.
func TestFlow_SkipEnvSkipsTheEnvScreen(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nAPI_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	detected := Detect(dir)
	require.True(t, detected.HasEnvFile())

	model := press(t, pastName(t, newFlow(detected, nil, Answers{SkipEnv: true})), "enter", "enter", "enter")

	assert.Equal(t, screenConfirm, model.at, "the env screen is not a question the user left open")
	assert.Empty(t, model.draft.EnvVars)
	assert.NotContains(t, string(model.content), "environmentVars")

	// Without the flag the same project stops at the env screen.
	withEnv := press(t, pastName(t, newFlow(detected, nil, Answers{})), "enter", "enter", "enter")
	assert.Equal(t, screenEnv, withEnv.at)
}

// A workload that scales itself has no replica count to give, and the screen
// must not demand one: applyRuntime would drop the answer anyway.
func TestFlow_AutoscaledBoundWorkloadPassesTheSettingsScreen(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "triage-agent", "importance": "low", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default",
				"autoscaling": {"enabled": true, "minReplicaCount": 2, "maxReplicaCount": 9},
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "triage-agent-artifact", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9001, "imageUri": "registry/triage:v3"}]}]}}`))

	model := newFlow(dockerfileProject(t), nil, Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"})

	updated, _ := model.Update(liveLoadedMsg{live: mustFetchLive(t, "68b0c1d2e3f4a5b6c7d8e9f0")})
	model, ok := updated.(flow)
	require.True(t, ok)

	require.True(t, model.autoscaled())

	model = press(t, model, "enter") // kind
	model = press(t, model, "enter") // image source
	model = press(t, model, "enter") // keep the live image
	require.Equal(t, screenSettings, model.at)

	// The field says why it is empty rather than showing a zero nobody typed.
	assert.Empty(t, model.inputs[2].Value())
	assert.Contains(t, model.inputs[2].Placeholder, "autoscaling")

	model = press(t, model, "enter")
	require.NoError(t, model.failed, "the screen must not block on a count the file cannot carry")
	assert.Equal(t, screenConfirm, model.at)

	// The policy survives untouched.
	assert.Contains(t, string(model.content), "maxReplicaCount: 9")
	assert.NotContains(t, string(model.content), "replicaCount: 1")
}

// ParseFloat takes NaN and Inf, and neither is <= 0, so a plain positivity
// check lets both through to a file nothing can schedule.
func TestFlow_SettingsRejectsNonFiniteCPU(t *testing.T) {
	for _, text := range []string{"NaN", "+Inf", "-Inf", "inf"} {
		t.Run(text, func(t *testing.T) {
			model := press(t, pastName(t, newFlow(dockerfileProject(t), nil, Answers{})), "enter", "enter")
			require.Equal(t, screenSettings, model.at)

			model.inputs[3].SetValue(text)
			model = press(t, model, "enter")

			require.Error(t, model.failed)
			assert.Contains(t, model.failed.Error(), "cpu must be a positive number")
			assert.Equal(t, screenSettings, model.at)
		})
	}
}
