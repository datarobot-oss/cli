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

package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serviceDraft is the Dockerfile happy path: what a user gets by pressing
// Enter through every screen in a directory that has a Dockerfile.
func serviceDraft() Draft {
	return Draft{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Name:       "my-app",
		Type:       TypeService,
		Build:      Build{Mode: BuildModeDockerfile},
		Port:       DefaultPort,
		HealthPath: DefaultHealthPath,
		Runtime:    Runtime{Replicas: DefaultReplicas, CPU: DefaultCPU, Memory: DefaultMemory},
	}
}

// TestRender_DockerfileService pins the whole file, not a field at a time:
// the confirm screen shows these exact bytes, so a change in layout, comments
// or key order is a change to what the user reads and agrees to.
func TestRender_DockerfileService(t *testing.T) {
	out, err := serviceDraft().Render()
	require.NoError(t, err)

	assert.Equal(t, `workloadId: 68b0c1d2e3f4a5b6c7d8e9f0 # managed by the CLI
name: my-app
importance: low # low | moderate | high | critical

artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080 # must be 1024 or above
            imageBuildConfig: # built from this repository's Dockerfile
              dockerfile:
                source: provided
            readinessProbe:
              path: /health
              port: 8080

runtime:
  containerGroups:
    - name: default # matches the artifact group
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`, string(out))
}

// An unbound draft writes no workloadId at all, so the first up knows the
// workload has to be created rather than found.
func TestRender_UnboundOmitsWorkloadID(t *testing.T) {
	draft := serviceDraft()
	draft.WorkloadID = ""

	out, err := draft.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(out), keyWorkloadID)
	assert.Contains(t, string(out), "name: my-app\n")
}

func TestRender_GeneratedAgent(t *testing.T) {
	draft := serviceDraft()
	draft.Type = TypeAgent
	draft.A2AEnabled = true
	draft.Build = Build{
		Mode:                          BuildModeGenerated,
		ExecutionEnvironmentID:        "68a1b2c3d4e5f6a7b8c9d0e1",
		ExecutionEnvironmentVersionID: "68a1b2c3d4e5f6a7b8c9d0e2",
		Entrypoint:                    []string{"uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080"},
	}

	out, err := draft.Render()
	require.NoError(t, err)

	assert.Contains(t, string(out), "type: agent")
	assert.Contains(t, string(out), "a2aEnabled: true")
	assert.Contains(t, string(out), "source: generated")
	assert.Contains(t, string(out), "executionEnvironmentId: 68a1b2c3d4e5f6a7b8c9d0e1")
	assert.Contains(t, string(out), "executionEnvironmentVersionId: 68a1b2c3d4e5f6a7b8c9d0e2")
	// Quoted throughout, so an entrypoint reads as the command line it is.
	assert.Contains(t, string(out), `entrypoint: ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080"]`)
}

// An agent that does not serve an A2A card writes only the type: false is the
// platform's default and saying it adds nothing.
func TestRender_AgentWithoutA2A(t *testing.T) {
	draft := serviceDraft()
	draft.Type = TypeAgent

	out, err := draft.Render()
	require.NoError(t, err)

	assert.Contains(t, string(out), "type: agent")
	assert.NotContains(t, string(out), "a2aEnabled")
}

func TestRender_PublishedImage(t *testing.T) {
	draft := serviceDraft()
	draft.Build = Build{Mode: BuildModeImage, ImageURI: "registry.example.com/team/my-app:v1"}

	out, err := draft.Render()
	require.NoError(t, err)

	assert.Contains(t, string(out), "imageUri: registry.example.com/team/my-app:v1")
	assert.NotContains(t, string(out), keyImageBuildConfig)
}

func TestRender_UnknownBuildMode(t *testing.T) {
	draft := serviceDraft()
	draft.Build = Build{Mode: "magic"}

	_, err := draft.Render()
	require.Error(t, err)

	assert.Contains(t, err.Error(), `unknown build mode "magic"`)
}

// A name that YAML would read back as a number or a boolean comes out quoted,
// so the file still says what the user typed.
func TestRender_QuotesAmbiguousNames(t *testing.T) {
	for _, name := range []string{"8080", "yes", "null"} {
		t.Run(name, func(t *testing.T) {
			draft := serviceDraft()
			draft.Name = name

			out, err := draft.Render()
			require.NoError(t, err)

			assert.Contains(t, string(out), `name: "`+name+`"`)

			m, err := Parse(out, "")
			require.NoError(t, err)
			assert.Equal(t, name, m.Name())
		})
	}
}

// Everything Render writes has to survive the reader that consumes it: a
// generated file parses, validates clean, and compiles to a create payload.
func TestRender_RoundTripsThroughValidate(t *testing.T) {
	generated := Build{
		Mode:                          BuildModeGenerated,
		ExecutionEnvironmentID:        "68a1b2c3d4e5f6a7b8c9d0e1",
		ExecutionEnvironmentVersionID: "68a1b2c3d4e5f6a7b8c9d0e2",
		Entrypoint:                    []string{"python", "main.py"},
	}

	for name, build := range map[string]Build{
		"dockerfile": {Mode: BuildModeDockerfile},
		"generated":  generated,
		"image":      {Mode: BuildModeImage, ImageURI: "nginx:latest"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, dockerfileName), []byte("FROM scratch\n"), 0o600))

			draft := serviceDraft()
			draft.Build = build

			out, err := draft.Render()
			require.NoError(t, err)

			m, err := Parse(out, dir)
			require.NoError(t, err)
			require.NoError(t, m.Validate())

			compiled, err := m.Compile()
			require.NoError(t, err)

			assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", compiled.WorkloadID)
			assert.NotContains(t, string(compiled.Payload), keyWorkloadID)
		})
	}
}

func TestWrite_CreatesFile(t *testing.T) {
	path := Path(t.TempDir())

	require.NoError(t, Write(path, []byte("name: my-app\n")))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name: my-app\n", string(got))

	if runtime.GOOS != "windows" {
		// Compared against a file the standard library just wrote the same
		// way, so the assertion holds under any umask and the manifest is
		// never wider than what the user's own tools produce.
		reference := filepath.Join(t.TempDir(), "reference")
		require.NoError(t, os.WriteFile(reference, nil, 0o644))

		want, err := os.Stat(reference)
		require.NoError(t, err)

		got, err := os.Stat(path)
		require.NoError(t, err)

		assert.Equal(t, want.Mode().Perm(), got.Mode().Perm())
	}
}

// A write that fails part way must leave no file at all: the refuse-to-
// overwrite rule would otherwise make a truncated manifest unrepairable by
// the CLI that produced it.
func TestWrite_FailureLeavesNoFile(t *testing.T) {
	original := atomicWrite
	atomicWrite = func(string, []byte) error { return errors.New("disk full") }

	t.Cleanup(func() { atomicWrite = original })

	path := Path(t.TempDir())

	err := Write(path, []byte("name: my-app\n"))
	require.Error(t, err)

	assert.NoFileExists(t, path, "a failed write must not leave a manifest behind")
}

// Setup never rewrites a manifest: the file is the interface after the first
// write, so a second config against the same directory has to stop.
func TestWrite_RefusesExisting(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest)

	err := Write(path, []byte("name: replaced\n"))
	require.ErrorIs(t, err, ErrExists)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, validManifest, string(got), "the existing manifest must be left alone")
}

func TestWriteWorkloadID_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest)

	require.NoError(t, WriteWorkloadID(path, "68b0ffff0000000000000009"))

	m, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0ffff0000000000000009", m.WorkloadID())
}

// The write-back is the one way into this package that does not go through
// Parse, so it runs the alias guard itself rather than inheriting one. Nothing
// in it expands an alias today; the guard is what keeps that from being a
// property of two other functions that nobody is watching.
func TestWriteWorkloadID_RejectsAnAliasBomb(t *testing.T) {
	path := writeManifest(t, t.TempDir(), aliasBomb)

	err := WriteWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "too large")

	// The file is the user's, and a refusal must not have edited it.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, aliasBomb, string(got))
}

// The write-back is a single-key edit, not a regeneration: comments, unknown
// keys and their order are the user's, and up leaves them alone.
func TestWriteWorkloadID_PreservesTheRestOfTheFile(t *testing.T) {
	original := `name: my-app          # the workload
somethingTheApiAddedLater: true
artifact:
  # how the image is built
  name: my-app-artifact
  spec:
    type: service
`

	path := writeManifest(t, t.TempDir(), original)

	require.NoError(t, WriteWorkloadID(path, "68b0ffff0000000000000009"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, `workloadId: 68b0ffff0000000000000009 # managed by the CLI
name: my-app # the workload
somethingTheApiAddedLater: true
artifact:
  # how the image is built
  name: my-app-artifact
  spec:
    type: service
`, string(got))
}

// A banner at the top of the file belongs to the file, not to the key that
// happened to be first, so the insert goes underneath it.
func TestWriteWorkloadID_KeepsFileBannerOnTop(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `# Deploy config for my-app. Edit freely.
name: my-app
`)

	require.NoError(t, WriteWorkloadID(path, "68b0ffff0000000000000009"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, `# Deploy config for my-app. Edit freely.
workloadId: 68b0ffff0000000000000009 # managed by the CLI
name: my-app
`, string(got))
}

func TestWriteWorkloadID_RejectsNonMapping(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "- name: my-app\n")

	err := WriteWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "root must be a YAML mapping")
}

func TestWriteWorkloadID_MissingFile(t *testing.T) {
	err := WriteWorkloadID(Path(t.TempDir()), "68b0ffff0000000000000009")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "cannot read")
}

func TestArtifactName(t *testing.T) {
	assert.Equal(t, "my-app-artifact", ArtifactName("my-app"))
}

// A cpu allocation is written as the number it is: plainly when whole, with
// its decimals otherwise, and never with a !!float tag on a value that reads
// as an integer. The last two rows are far outside anything a resource bundle
// offers and are here to show the rule needs no magnitude bound to hold: the
// digits decide, so there is no conversion that could quietly return a
// different number.
func TestDecimal(t *testing.T) {
	for _, tc := range []struct {
		in        float64
		tag, want string
	}{
		{0.25, "!!float", "0.25"},
		{0.5, "!!float", "0.5"},
		{1, intTag, "1"},
		{2, intTag, "2"},
		{1.5, "!!float", "1.5"},
		{1e15, intTag, "1000000000000000"},
		{1e19, intTag, "10000000000000000000"},
	} {
		node := decimal(tc.in)

		assert.Equal(t, tc.tag, node.Tag, "tag for %v", tc.in)
		assert.Equal(t, tc.want, node.Value, "value for %v", tc.in)
	}
}

// A value that is not a secret is written as the literal it is. A secret is
// written as a reference carrying the placeholder, so the entry is present
// and visibly unfinished: a deploy then fails naming the variable, where a
// missing entry would deploy cleanly and leave the container without it.
func TestRender_SecretsCarryThePlaceholder(t *testing.T) {
	draft := serviceDraft()
	draft.EnvVars = []EnvVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "OPENAI_API_KEY", Secret: true},
	}

	out, err := draft.Render()
	require.NoError(t, err)

	assert.Contains(t, string(out), "- name: LOG_LEVEL\n                value: debug")
	assert.Contains(t, string(out), "- name: OPENAI_API_KEY\n                value: dr-credential:PLACEHOLDER/apiToken")
	assert.Contains(t, string(out), "# replace PLACEHOLDER with the credential id")

	// Nothing is commented out: the whole block is live.
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "OPENAI_API_KEY") {
			assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "#"), "the entry is live: %q", line)
		}
	}

	m, err := Parse(out, "")
	require.NoError(t, err)
	require.NoError(t, m.Validate(), "a placeholder is syntactically a reference, so the file is writable")

	compiled, err := m.Compile()
	require.NoError(t, err)

	assert.Contains(t, string(compiled.Payload), "LOG_LEVEL")

	// The reference is collected, so the deploy resolves it and fails naming
	// the variable rather than starting a container without its secret.
	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, CredentialPlaceholder, compiled.CredentialRefs[0].CredentialID)
	assert.Equal(t, "OPENAI_API_KEY", compiled.CredentialRefs[0].EnvName)
}

func TestRender_NoEnvKeysWritesNoBlock(t *testing.T) {
	out, err := serviceDraft().Render()
	require.NoError(t, err)

	assert.NotContains(t, string(out), "environmentVars")
	assert.NotContains(t, string(out), ".env")
}

// Two secrets render the same placeholder text, differing only in which entry
// they sit in, so the id has to be matched to the variable by name. A textual
// substitution could not tell them apart and would give both the same
// credential.
func TestResolveCredentials_MatchesEachSecretByName(t *testing.T) {
	content := []byte(`name: my-app
artifact:
  name: my-app-artifact
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            environmentVars:
              - name: OPENAI_API_KEY
                value: dr-credential:PLACEHOLDER/apiToken # replace PLACEHOLDER with the credential id
              - name: DATABASE_URL
                value: dr-credential:PLACEHOLDER/apiToken # replace PLACEHOLDER with the credential id
              - name: LOG_LEVEL
                value: debug
`)

	out, err := ResolveCredentials(content, map[string]string{
		"OPENAI_API_KEY": "66f000000000000000000001",
		"DATABASE_URL":   "66f000000000000000000002",
	})
	require.NoError(t, err)

	got := string(out)

	assert.Contains(t, got, "dr-credential:66f000000000000000000001/apiToken")
	assert.Contains(t, got, "dr-credential:66f000000000000000000002/apiToken")
	assert.NotContains(t, got, CredentialPlaceholder, "no entry keeps a placeholder, and none is told to replace one")
	assert.Contains(t, got, "value: debug", "a variable that is not a secret is untouched")
}

// An entry no credential was stored for keeps its placeholder: it is the file
// saying, visibly, that it is not finished.
func TestResolveCredentials_LeavesUnstoredEntriesAlone(t *testing.T) {
	content := []byte(`name: my-app
artifact:
  name: my-app-artifact
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            environmentVars:
              - name: OPENAI_API_KEY
                value: dr-credential:PLACEHOLDER/apiToken
              - name: DATABASE_URL
                value: dr-credential:PLACEHOLDER/apiToken
`)

	out, err := ResolveCredentials(content, map[string]string{"OPENAI_API_KEY": "66f000000000000000000001"})
	require.NoError(t, err)

	got := string(out)

	assert.Contains(t, got, "dr-credential:66f000000000000000000001/apiToken")
	assert.Contains(t, got, "dr-credential:PLACEHOLDER/apiToken", "the one nothing was stored for stays unfinished")
}

// A hand-edited file can be anything, including unparseable. Failing loudly
// beats writing a file the ids never reached.
func TestResolveCredentials_RefusesWhatItCannotParse(t *testing.T) {
	_, err := ResolveCredentials([]byte("name: [unclosed\n"), map[string]string{"A": "b"})
	require.Error(t, err)
}
