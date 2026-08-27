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
// Enter through every screen in a directory that has a Dockerfile, plus a
// probe typed on the settings screen: no probe is written by default, and
// this fixture is what pins the render of one that was asked for.
func serviceDraft() Draft {
	return Draft{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Name:       "my-app",
		Type:       TypeService,
		Build:      Build{Mode: BuildModeDockerfile},
		Port:       DefaultPort,
		HealthPath: "/",
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
  type: service
  spec:
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
              path: /
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

// A readiness probe is optional to the platform, and one aimed at an endpoint
// the container does not serve never passes. No path means no block: the
// alternative is a probe with an empty path, which is neither.
func TestRender_NoHealthPathWritesNoProbe(t *testing.T) {
	draft := serviceDraft()
	draft.HealthPath = ""

	out, err := draft.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(out), keyReadinessProbe)
	// The container's own port is unaffected: it is what traffic arrives on,
	// not something the probe brought with it.
	assert.Contains(t, string(out), "port: 8080")
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
  type: service
  spec:
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
  type: service
  spec:
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

func TestClearWorkloadID_RemovesTheBinding(t *testing.T) {
	path := writeManifest(t, t.TempDir(), validManifest)

	m, err := Load(path)
	require.NoError(t, err)
	require.NotEmpty(t, m.WorkloadID(), "the fixture has to start bound for this to mean anything")

	require.NoError(t, clearOK(t, path))

	cleared, err := Load(path)
	require.NoError(t, err)
	assert.Empty(t, cleared.WorkloadID())

	// The rest of the manifest still has to parse and validate: clearing the
	// binding returns the file to a project that has never deployed, not to a
	// broken one.
	require.NoError(t, cleared.Validate())
}

// Clearing is the same single-key edit the write-back is, in the other
// direction: everything the user owns survives it.
func TestClearWorkloadID_PreservesTheRestOfTheFile(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `workloadId: 68b0ffff0000000000000009 # managed by the CLI
name: my-app          # the workload
somethingTheApiAddedLater: true
artifact:
  # how the image is built
  name: my-app-artifact
  type: service
  spec:
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, `name: my-app # the workload
somethingTheApiAddedLater: true
artifact:
  # how the image is built
  name: my-app-artifact
  type: service
  spec:
`, string(got))
}

// The mirror of KeepsFileBannerOnTop: the write-back moved the banner up onto
// the key it inserted, so removing that key has to hand it back rather than
// take the file's own header with it.
func TestClearWorkloadID_KeepsFileBannerOnTop(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `# Deploy config for my-app. Edit freely.
workloadId: 68b0ffff0000000000000009 # managed by the CLI
name: my-app
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, `# Deploy config for my-app. Edit freely.
name: my-app
`, string(got))
}

// A comment further down describes the key it sits on, so it goes when that
// key goes. Only the first key's head comment is the file's.
func TestClearWorkloadID_DropsACommentOnItsOwnKey(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `name: my-app
# the workload this checkout deploys to
workloadId: 68b0ffff0000000000000009
importance: low
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, `name: my-app
importance: low
`, string(got))
}

// A manifest with no binding is the ordinary state of a project that has never
// deployed, so clearing one is a no-op rather than an error, and it must not
// rewrite the file: re-emitting would reformat a file nobody asked it to.
func TestClearWorkloadID_AbsentKeyLeavesTheFileByteForByte(t *testing.T) {
	original := "name:    my-app\nimportance:   low\n"
	path := writeManifest(t, t.TempDir(), original)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

// A hand-edited file can hold two bindings: the reader returns the first match
// and never looks further, so the second is invisible to it. Clearing only the
// one it reports would leave the next deploy bound to what was behind it.
func TestClearWorkloadID_RemovesEveryOccurrence(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `workloadId: 68b0ffff0000000000000009
name: my-app
workloadId: 68b0ffff0000000000000009
`)

	before, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "68b0ffff0000000000000009", before.WorkloadID(),
		"the reader stops at the first match, so the second binding is the one nothing reports")

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.NoError(t, err)
	assert.True(t, cleared)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name: my-app\n", string(got),
		"removing only the one the reader reports would leave the deploy still bound")
}

// Repeated keys carrying different ids are a file nothing can reason about:
// the reader reports one, and removing it would promote the other to the
// binding without anyone asking. Refusing leaves the ambiguity visible.
func TestClearWorkloadID_RefusesDisagreeingDuplicates(t *testing.T) {
	original := `workloadId: 68b0ffff0000000000000009
name: my-app
workloadId: 68b0aaaa0000000000000001
`
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "different values")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestClearWorkloadID_RejectsAnAliasBomb(t *testing.T) {
	path := writeManifest(t, t.TempDir(), aliasBomb)

	_, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "too large")
	require.ErrorIs(t, err, ErrUnreadable,
		"a file that cannot be walked safely cannot be judged relevant either")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, aliasBomb, string(got))
}

func TestClearWorkloadID_RejectsNonMapping(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "- name: my-app\n")

	_, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "root must be a YAML mapping")
}

func TestClearWorkloadID_MissingFile(t *testing.T) {
	_, err := ClearWorkloadID(Path(t.TempDir()), "68b0aaaa0000000000000001")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "cannot read")
}

// Written and cleared leaves the file it started as, which is the property
// that makes the pair safe to run in either order.
func TestWorkloadID_WriteThenClearRoundTrips(t *testing.T) {
	original := `# Deploy config for my-app. Edit freely.
name: my-app # the workload
somethingTheApiAddedLater: true
`

	path := writeManifest(t, t.TempDir(), original)

	require.NoError(t, WriteWorkloadID(path, "68b0ffff0000000000000009"))
	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
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

// The platform pops spec.type on the way in and re-derives the discriminator
// from the artifact's own type, defaulting to service. A type written one
// level down is therefore discarded without a word: an agent silently becomes
// a service, and a2aEnabled is then rejected as an unknown field.
func TestRender_TypeSitsOnTheArtifactNotInTheSpec(t *testing.T) {
	draft := serviceDraft()
	draft.Type = TypeAgent
	draft.A2AEnabled = true

	rendered, err := draft.Render()
	require.NoError(t, err)

	out := string(rendered)

	assert.Contains(t, out, "  name: my-app-artifact\n  type: agent\n  spec:\n")
	assert.NotContains(t, out, "    type: agent", "inside the spec the platform throws it away")
	assert.Contains(t, out, "a2aEnabled: true", "which stays in the spec, where the agent variant reads it")
}

// The type is compared, not just defaulted, and folding is the whole point:
// the platform's own documents disagree about the casing of their enums, so an
// exact comparison turns a spelling into drift nothing can satisfy.
func TestSameArtifactType(t *testing.T) {
	assert.True(t, SameArtifactType("service", "service"))
	assert.True(t, SameArtifactType("Service", "service"), "casing is not a change")
	assert.True(t, SameArtifactType("AGENT", "agent"))
	assert.False(t, SameArtifactType("agent", "service"), "a different kind still is")
}

func TestArtifactTypeOrDefault(t *testing.T) {
	assert.Equal(t, TypeService, ArtifactTypeOrDefault(""),
		"the platform creates a block that names no type as a service")
	assert.Equal(t, TypeAgent, ArtifactTypeOrDefault(TypeAgent))
}

// clearOK clears whatever binding the file holds, which is what every fixture
// here wants: the compare-and-clear contract is exercised on its own below.
func clearOK(t *testing.T, path string) error {
	t.Helper()

	_, err := ClearWorkloadID(path, idIn(t, path))

	return err
}

// idIn reads the binding a fixture was written with.
func idIn(t *testing.T, path string) string {
	t.Helper()

	m, err := Load(path)
	require.NoError(t, err)

	return m.WorkloadID()
}

// The id is compared inside the same parse that removes it, so a binding that
// is not the one the caller named is left alone. Checking against a separate
// read would let a concurrent up write a fresh binding into the window and
// have this delete an id it never looked at.
func TestClearWorkloadID_LeavesADifferentBinding(t *testing.T) {
	path := writeManifest(t, t.TempDir(), validManifest)

	cleared, err := ClearWorkloadID(path, "68b0ffffffffffffffffffff")
	require.NoError(t, err)
	assert.False(t, cleared)

	m, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0aaaa0000000000000001", m.WorkloadID())
}

// setWorkloadID moves a file banner up onto the key it inserts. Removing that
// key has to give the banner back without overwriting a comment the surviving
// key already carries, because both belong to the user.
func TestClearWorkloadID_KeepsBothTheBannerAndTheNextKeysComment(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `# Deploy config for my-app. Edit freely.
workloadId: 68b0ffff0000000000000009 # managed by the CLI
# what this thing is called in the UI
name: my-app
importance: low
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Contains(t, string(got), "# Deploy config for my-app. Edit freely.")
	assert.Contains(t, string(got), "# what this thing is called in the UI",
		"a comment on the surviving key is the user's and must not be overwritten")
}

// A comment under the binding describes what follows it, not the binding.
func TestClearWorkloadID_KeepsAFootComment(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `name: my-app
workloadId: 68b0ffff0000000000000009
# remember to bump replicas before the demo
importance: low
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "# remember to bump replicas before the demo")
}

// A manifest is one document. Editing a file that carries more cannot put the
// rest back as it found them, because yaml.v3 hangs a comment that opens the
// second document off the end of the first, so re-emitting moves the user's
// text between documents. Refusing says so rather than rearranging it.
func TestClearWorkloadID_RefusesAMultiDocumentFile(t *testing.T) {
	original := `workloadId: 68b0ffff0000000000000009
name: my-app
---
# a note the user keeps
name: notes-the-user-keeps-here
`
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "2 YAML documents")
	require.NotErrorIs(t, err, ErrUnreadable,
		"the file parsed, so this is a refusal to edit and the caller has to be able to say so")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "a refusal must not have moved anything")
}

// Emptying the mapping would re-emit as "{}", which Parse then rejects as not
// a manifest, and because the file still exists nothing falls back to the
// wizard: every later deploy in that directory fails on a file the CLI wrote.
func TestClearWorkloadID_RefusesToEmptyTheFile(t *testing.T) {
	original := "workloadId: 68b0ffff0000000000000009\n"
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "only key")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "a refusal must not have edited the file")
}

// An unreadable file is distinguishable from an edit that failed after the
// file was understood, so a caller editing a file it merely found can stay
// quiet about one and speak up about the other.
func TestClearWorkloadID_UnreadableIsItsOwnError(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "name: [unclosed\n")

	_, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnreadable)
}

// The write-back runs on every deploy, so it is the likelier of the two edits
// to meet a file carrying more than one document. It refuses for the same
// reason, rather than silently dropping or rearranging what it cannot restore.
func TestWriteWorkloadID_RefusesAMultiDocumentFile(t *testing.T) {
	original := `name: my-app
---
name: notes-the-user-keeps-here
`
	path := writeManifest(t, t.TempDir(), original)

	err := WriteWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 YAML documents")
	require.NotErrorIs(t, err, ErrUnreadable,
		"the write-back knows its own file; tagging it unreadable makes its caller contradict itself")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

// Removing a binding whose value something else aliases would strand the
// alias, and the file would never load again.
func TestClearWorkloadID_RefusesAnAliasedBinding(t *testing.T) {
	original := "workloadId: &wid 68b0ffff0000000000000009\nname: my-app\nimportance: *wid\n"
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "anchor")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))

	// The point of refusing: the file still loads.
	_, err = Load(path)
	require.NoError(t, err)
}

// A binding written as an alias is still this project's binding, because that
// is how the reader resolves it. Matching on the raw node reads the anchor's
// name, agrees with nothing, and leaves `delete` reporting success over a file
// as stale as it found it.
func TestClearWorkloadID_ClearsABindingWrittenAsAnAlias(t *testing.T) {
	path := writeManifest(t, t.TempDir(),
		"bindings:\n  primary: &wid 68b0ffff0000000000000009\nworkloadId: *wid\nname: my-app\n")

	// The reader's own answer, which is what the delete compares against.
	m, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "68b0ffff0000000000000009", m.WorkloadID())

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.NoError(t, err)
	assert.True(t, cleared)

	after, err := Load(path)
	require.NoError(t, err)
	assert.Empty(t, after.WorkloadID())
}

// The other direction of the same rule: an alias that resolves to a different
// id is a different project's binding and is left alone.
func TestClearWorkloadID_LeavesAnAliasResolvingElsewhere(t *testing.T) {
	original := "bindings:\n  primary: &wid 68b0ffff0000000000000009\nworkloadId: *wid\nname: my-app\n"
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0aaaa0000000000000001")
	require.NoError(t, err)
	assert.False(t, cleared)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

// An anchor nothing refers to is not a reason to refuse.
func TestClearWorkloadID_AllowsAnUnreferencedAnchor(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "workloadId: &wid 68b0ffff0000000000000009\nname: my-app\n")

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.NoError(t, err)
	assert.True(t, cleared)
}

// Recognized keys are what Parse uses to decide a file is a manifest at all,
// so a survivor set carrying none of them is a file every later command fails
// on. workloadId is itself recognized, which is what makes this reachable.
func TestClearWorkloadID_RefusesWhenNoRecognizedKeySurvives(t *testing.T) {
	original := "workloadId: 68b0ffff0000000000000009\nsomethingElse: true\n"
	path := writeManifest(t, t.TempDir(), original)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "Delete the file instead",
		"the remedy for this one is not to remove a line")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

// A comment at the bottom of the file stays at the bottom. Hoisting it to the
// first key is how an edit moves a user's text somewhere they never put it.
func TestClearWorkloadID_KeepsABottomCommentAtTheBottom(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `name: my-app
importance: low
workloadId: 68b0ffff0000000000000009
# a note at the very bottom
`)

	require.NoError(t, clearOK(t, path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name: my-app\nimportance: low\n# a note at the very bottom\n", string(got))
}

// Repeated keys that disagree are refused with a message, not silently: the
// caller prints what it is given, and "nothing happened" is indistinguishable
// from "no binding here".
func TestClearWorkloadID_DisagreeingDuplicatesAreObservable(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `workloadId: 68b0ffff0000000000000009
name: my-app
workloadId: 68b0aaaa0000000000000001
`)

	cleared, err := ClearWorkloadID(path, "68b0ffff0000000000000009")
	require.Error(t, err)
	assert.False(t, cleared)
	assert.Contains(t, err.Error(), "different values")
}
