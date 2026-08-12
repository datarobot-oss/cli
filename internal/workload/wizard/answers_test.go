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
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExecEnv points the resolver at a fixed answer for one test.
func stubExecEnv(t *testing.T, id, versionID string, err error) {
	t.Helper()

	original := resolveExecEnvFn
	resolveExecEnvFn = func(string) (string, string, error) { return id, versionID, err }

	t.Cleanup(func() { resolveExecEnvFn = original })
}

func TestSplitCommand(t *testing.T) {
	tests := map[string]struct {
		command string
		want    []string
	}{
		"plain":            {"python main.py", []string{"python", "main.py"}},
		"extra spaces":     {"  python   main.py  ", []string{"python", "main.py"}},
		"double quotes":    {`sh -c "echo hello world"`, []string{"sh", "-c", "echo hello world"}},
		"single quotes":    {`sh -c 'echo $HOME'`, []string{"sh", "-c", "echo $HOME"}},
		"escaped space":    {`/opt/my\ app/run`, []string{"/opt/my app/run"}},
		"empty quoted arg": {`run ""`, []string{"run", ""}},
		"quote inside":     {`echo "it's here"`, []string{"echo", "it's here"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := splitCommand(test.command)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSplitCommand_Rejects(t *testing.T) {
	tests := map[string]string{
		"unbalanced quote":   `sh -c "echo hello`,
		"trailing backslash": `run \`,
		"empty":              "   ",
	}

	for name, command := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := splitCommand(command)
			require.Error(t, err)
		})
	}
}

func TestAnswers_Check(t *testing.T) {
	tests := map[string]struct {
		answers Answers
		want    string
	}{
		"id and name together": {
			Answers{WorkloadID: "68b0", Name: "my-app"}, "exclusive",
		},
		"unsupported type": {
			Answers{Type: "nim"}, `--type "nim" is not supported`,
		},
		"unsupported build mode": {
			Answers{BuildMode: "magic"}, `--build-mode "magic" is not supported`,
		},
		"health without a slash": {
			Answers{HealthPath: "health"}, "must start with /",
		},
		"privileged port": {
			Answers{Port: 80}, "--port 80 is too low",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.answers.check()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

// --a2a-enabled is judged against the kind that will be written rather than
// against the flag, so the check runs once the draft has a type. On a new
// workload that is the documented default; binding covers the other half.
func TestAnswers_A2AIsCheckedAgainstTheDraftType(t *testing.T) {
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\n"))

	_, err := Answers{Name: "my-app", A2AEnabled: true}.draft(detected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--a2a-enabled applies to --type agent")

	_, err = Answers{Name: "my-app", Type: manifest.TypeAgent, A2AEnabled: true}.draft(detected)
	require.NoError(t, err)
}

// The Dockerfile track needs no flags at all: this is the headless half of
// the Enter-through-every-screen path.
func TestAnswers_DraftFromDetectionAlone(t *testing.T) {
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n"))

	draft, err := Answers{}.draft(detected)
	require.NoError(t, err)

	assert.Equal(t, manifest.BuildModeDockerfile, draft.Build.Mode)
	assert.Equal(t, detected.Name, draft.Name)
	assert.Equal(t, 3000, draft.Port)
	assert.Equal(t, manifest.DefaultHealthPath, draft.HealthPath)
	assert.Equal(t, manifest.TypeService, draft.Type)
	assert.Equal(t, manifest.DefaultImportance, draft.Importance)
	assert.Equal(t, manifest.DefaultReplicas, draft.Runtime.Replicas)
	assert.InEpsilon(t, manifest.DefaultCPU, draft.Runtime.CPU, 0.0001)
	assert.Equal(t, manifest.DefaultMemory, draft.Runtime.Memory)
}

func TestAnswers_FlagsWinOverDetection(t *testing.T) {
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n"))

	draft, err := Answers{
		Name:       "chosen",
		Type:       manifest.TypeAgent,
		A2AEnabled: true,
		Port:       9999,
		HealthPath: "/healthz",
		Replicas:   3,
		CPU:        2,
		Memory:     "4GB",
	}.draft(detected)
	require.NoError(t, err)

	assert.Equal(t, "chosen", draft.Name)
	assert.Equal(t, manifest.TypeAgent, draft.Type)
	assert.True(t, draft.A2AEnabled)
	assert.Equal(t, 9999, draft.Port)
	assert.Equal(t, "/healthz", draft.HealthPath)
	assert.Equal(t, 3, draft.Runtime.Replicas)
	assert.InEpsilon(t, 2.0, draft.Runtime.CPU, 0.0001)
	assert.Equal(t, "4GB", draft.Runtime.Memory)
}

// With nothing to build and nothing said, the error names the flags that
// would settle it rather than guessing at an image source.
func TestAnswers_NoDockerfileAndNoFlags(t *testing.T) {
	_, err := Answers{}.draft(Detect(t.TempDir()))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--build-mode image")
	assert.Contains(t, err.Error(), "--build-mode generated")
}

func TestAnswers_ImageMode(t *testing.T) {
	detected := Detect(t.TempDir())

	// A lone --image says which track this is; --build-mode is not needed.
	draft, err := Answers{Image: "registry/team/app:v1"}.draft(detected)
	require.NoError(t, err)

	assert.Equal(t, manifest.BuildModeImage, draft.Build.Mode)
	assert.Equal(t, "registry/team/app:v1", draft.Build.ImageURI)

	_, err = Answers{BuildMode: manifest.BuildModeImage}.draft(detected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--image is required")
}

func TestAnswers_GeneratedMode(t *testing.T) {
	stubExecEnv(t, "68a1", "68a2", nil)

	detected := Detect(t.TempDir())

	draft, err := Answers{
		BuildMode:            manifest.BuildModeGenerated,
		ExecutionEnvironment: "[DataRobot] Python 3.12 Applications Base",
		Entrypoint:           `uvicorn app:app --host "0.0.0.0"`,
	}.draft(detected)
	require.NoError(t, err)

	assert.Equal(t, "68a1", draft.Build.ExecutionEnvironmentID)
	assert.Equal(t, "68a2", draft.Build.ExecutionEnvironmentVersionID)
	assert.Equal(t, []string{"uvicorn", "app:app", "--host", "0.0.0.0"}, draft.Build.Entrypoint)
}

func TestAnswers_GeneratedModeMissingFlags(t *testing.T) {
	_, err := Answers{BuildMode: manifest.BuildModeGenerated}.draft(Detect(t.TempDir()))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--execution-environment and --entrypoint are required")
}

// A base image that does not resolve fails at setup, where the user is
// looking at it, not at the first deploy.
func TestAnswers_GeneratedModeUnknownEnvironment(t *testing.T) {
	stubExecEnv(t, "", "", errors.New(`execution environment "nope" not found`))

	_, err := Answers{
		BuildMode:            manifest.BuildModeGenerated,
		ExecutionEnvironment: "nope",
		Entrypoint:           "python main.py",
	}.draft(Detect(t.TempDir()))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// A Dockerfile track asked for explicitly still needs a Dockerfile.
func TestAnswers_DockerfileModeWithoutOne(t *testing.T) {
	_, err := Answers{BuildMode: manifest.BuildModeDockerfile}.draft(Detect(t.TempDir()))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "needs a Dockerfile")
}

// Binding layers flags over the live values, and leaves alone what the
// command line did not mention.
func TestAnswers_ApplyToKeepsLiveDefaults(t *testing.T) {
	defaults := manifest.Draft{
		WorkloadID: "68b0",
		Name:       "live-app",
		Type:       manifest.TypeAgent,
		Port:       9000,
		HealthPath: "/live",
		Build:      manifest.Build{Mode: manifest.BuildModeImage, ImageURI: "registry/live:v9"},
	}

	// A Dockerfile in the checkout must not switch a running workload's build
	// mode on its own.
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\n"))

	draft, err := Answers{Port: 8081}.applyTo(defaults, detected)
	require.NoError(t, err)

	assert.Equal(t, 8081, draft.Port)
	assert.Equal(t, "live-app", draft.Name)
	assert.Equal(t, manifest.TypeAgent, draft.Type)
	assert.Equal(t, "/live", draft.HealthPath)
	assert.Equal(t, manifest.BuildModeImage, draft.Build.Mode)

	// An explicit build flag does switch it.
	draft, err = Answers{BuildMode: manifest.BuildModeDockerfile}.applyTo(defaults, detected)
	require.NoError(t, err)
	assert.Equal(t, manifest.BuildModeDockerfile, draft.Build.Mode)
}
