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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateString parses content and runs the ledger over it. dir anchors the
// file-existence rules; pass "" to skip them.
func validateString(t *testing.T, dir, content string) error {
	t.Helper()

	m, err := Parse([]byte(content), dir)
	require.NoError(t, err)

	return m.Validate()
}

// requireFindings asserts the exact set of findings, each as a line number
// and a fragment of its message. Line numbers are part of the contract: a
// finding that cannot point at the offending line is not much of a fix hint.
func requireFindings(t *testing.T, err error, want []FieldError) {
	t.Helper()

	require.Error(t, err)

	var validationErr *ValidationError

	require.ErrorAs(t, err, &validationErr)
	require.Len(t, validationErr.Errors, len(want), "findings: %v", validationErr.Errors)

	for i, expected := range want {
		got := validationErr.Errors[i]

		assert.Equal(t, expected.Line, got.Line, "finding %d line", i)
		assert.Equal(t, expected.Path, got.Path, "finding %d path", i)
		assert.Contains(t, got.Msg, expected.Msg, "finding %d message", i)
	}
}

func TestValidate_AcceptsValidManifest(t *testing.T) {
	require.NoError(t, validateString(t, "", validManifest))
}

// The everyday file from the design: a service built from the repository's
// Dockerfile, with the Dockerfile actually there.
func TestValidate_AcceptsRenderedDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, dockerfileName), []byte("FROM scratch\n"), 0o600))

	out, err := serviceDraft().Render()
	require.NoError(t, err)

	require.NoError(t, validateString(t, dir, string(out)))
}

func TestValidate_NameRequired(t *testing.T) {
	err := validateString(t, "", `artifactId: 68b0bbbb0000000000000002
name: ""
`)

	requireFindings(t, err, []FieldError{{Line: 2, Path: keyName, Msg: "is required"}})
}

func TestValidate_ArtifactBinding(t *testing.T) {
	tests := map[string]string{
		"neither": `name: my-app
importance: low
`,
		"both": `name: my-app
artifactId: 68b0bbbb0000000000000002
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateString(t, "", content)

			requireFindings(t, err, []FieldError{{Line: 1, Path: "", Msg: "exactly one of artifact"}})
		})
	}
}

func TestValidate_InlineArtifactNeedsSpec(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
`)

	requireFindings(t, err, []FieldError{{Line: 3, Path: "artifact.spec", Msg: "is required for an inline artifact"}})
}

func TestValidate_ContainerGroupsRequired(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
`)

	requireFindings(t, err, []FieldError{
		{Line: 5, Path: "artifact.spec.containerGroups", Msg: "at least one container group"},
	})
}

func TestValidate_ImageSource(t *testing.T) {
	tests := map[string]struct {
		container string
		want      FieldError
	}{
		"both": {
			container: `            imageUri: nginx:latest
            imageBuildConfig:
              dockerfile:
                source: provided
`,
			want: FieldError{Line: 14, Msg: "never both"},
		},
		"neither": {
			container: "",
			want:      FieldError{Line: 9, Msg: "must set either imageUri or imageBuildConfig"},
		},
		"typo in the build block": {
			container: `            imageBuildConfig:
              dockerfle:
                source: provided
`,
			want: FieldError{Line: 13, Msg: "imageBuildConfig must say how the image is built"},
		},
		"unknown source": {
			container: `            imageBuildConfig:
              dockerfile:
                source: magic
`,
			want: FieldError{Line: 14, Msg: `must be "provided"`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateString(t, "", serviceWithContainerBody(test.container))

			require.Error(t, err)

			var validationErr *ValidationError

			require.ErrorAs(t, err, &validationErr)
			require.Len(t, validationErr.Errors, 1, "findings: %v", validationErr.Errors)
			assert.Equal(t, test.want.Line, validationErr.Errors[0].Line)
			assert.Contains(t, validationErr.Errors[0].Msg, test.want.Msg)
		})
	}
}

// A provided build with nothing to build fails here rather than after the
// sync, the build queue and the wait.
func TestValidate_ProvidedBuildNeedsDockerfile(t *testing.T) {
	dir := t.TempDir()
	content := serviceWithContainerBody(`            imageBuildConfig:
              dockerfile:
                source: provided
`)

	err := validateString(t, dir, content)
	requireFindings(t, err, []FieldError{
		{Line: 14, Path: "artifact.spec.containerGroups[0].containers[0].imageBuildConfig.dockerfile", Msg: "no Dockerfile exists in"},
	})

	require.NoError(t, os.WriteFile(filepath.Join(dir, dockerfileName), []byte("FROM scratch\n"), 0o600))
	require.NoError(t, validateString(t, dir, content))
}

func TestValidate_GeneratedBuildNeedsEnvironmentAndEntrypoint(t *testing.T) {
	err := validateString(t, "", serviceWithContainerBody(`            imageBuildConfig:
              dockerfile:
                source: generated
`))

	base := "artifact.spec.containerGroups[0].containers[0].imageBuildConfig.dockerfile"
	requireFindings(t, err, []FieldError{
		{Line: 14, Path: base + ".executionEnvironmentId", Msg: "is required"},
		{Line: 14, Path: base + ".executionEnvironmentVersionId", Msg: "is required"},
		{Line: 14, Path: base + ".entrypoint", Msg: "at least one argument"},
	})
}

func TestValidate_PrimaryPortFloor(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 80
            imageUri: nginx:latest
`)

	requireFindings(t, err, []FieldError{
		{Line: 11, Path: "artifact.spec.containerGroups[0].containers[0].port", Msg: "must listen on 1024 or above"},
	})
}

func TestValidate_PrimaryPortRequired(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            imageUri: nginx:latest
`)

	requireFindings(t, err, []FieldError{
		{Line: 9, Path: "artifact.spec.containerGroups[0].containers[0].port", Msg: "is required on the primary container"},
	})
}

func TestValidate_NonPrimaryContainerIsPortless(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
          - name: metrics-bridge
            port: 9000
            imageUri: registry/metrics:v1
`)

	requireFindings(t, err, []FieldError{
		{Line: 14, Path: "artifact.spec.containerGroups[0].containers[1].port", Msg: "only allowed on the primary container"},
	})
}

func TestValidate_OnlyOnePrimaryPerGroup(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
          - name: also-primary
            primary: true
            port: 9000
            imageUri: registry/other:v1
`)

	requireFindings(t, err, []FieldError{
		{Line: 7, Path: "artifact.spec.containerGroups[0]", Msg: "only one can be"},
	})
}

func TestValidate_ScalingIsEitherOr(t *testing.T) {
	err := validateString(t, "", runtimeWithGroupBody(`      replicaCount: 3
      autoscaling:
        enabled: true
        minReplicaCount: 1
        maxReplicaCount: 4
`))

	requireFindings(t, err, []FieldError{
		{Line: 16, Path: "runtime.containerGroups[0].replicaCount", Msg: "cannot be set alongside autoscaling"},
	})
}

// Autoscaling switched off is not scaling, so replicaCount still decides. The
// create endpoint accepts this, and the ledger must not be stricter than the
// thing it is protecting.
func TestValidate_DisabledAutoscalingAllowsReplicaCount(t *testing.T) {
	require.NoError(t, validateString(t, "", runtimeWithGroupBody(`      replicaCount: 3
      autoscaling:
        enabled: false
`)))
}

func TestValidate_RuntimeNamesMustMatchTheArtifact(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
runtime:
  containerGroups:
    - name: dfault
      replicaCount: 1
      containers:
        - name: primry
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`)

	requireFindings(t, err, []FieldError{
		{Line: 15, Path: "runtime.containerGroups[0].name", Msg: `"dfault" matches no artifact container group`},
	})
}

// A container name is only checkable once its group matched; reporting both
// for one typo would send the reader chasing a second problem that is not
// there.
func TestValidate_RuntimeContainerNameMustMatch(t *testing.T) {
	err := validateString(t, "", runtimeWithGroupBody(`      replicaCount: 1
      containers:
        - name: primry
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`))

	requireFindings(t, err, []FieldError{
		{Line: 18, Path: "runtime.containerGroups[0].containers[0].name", Msg: `"primry" matches no container in artifact group "default"`},
	})
}

// A manifest that binds an existing artifact by id has no local names to
// match against, so the runtime block is taken at its word.
func TestValidate_RuntimeNamesUncheckedForArtifactByID(t *testing.T) {
	require.NoError(t, validateString(t, "", `name: my-app
artifactId: 68b0bbbb0000000000000002
runtime:
  containerGroups:
    - name: whatever-the-artifact-calls-it
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`))
}

func TestValidate_MemoryUnits(t *testing.T) {
	accepted := []string{"512MB", "4GB", "1000B", "1048576"}
	for _, value := range accepted {
		t.Run("accepts "+value, func(t *testing.T) {
			require.NoError(t, validateString(t, "", runtimeWithGroupBody(`      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: `+value+`}
`)))
		})
	}

	rejected := []string{"4Gi", "512Mi", "big", "0.5GB"}
	for _, value := range rejected {
		t.Run("rejects "+value, func(t *testing.T) {
			err := validateString(t, "", runtimeWithGroupBody(`      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: `+value+`}
`))

			requireFindings(t, err, []FieldError{
				{Line: 19, Path: "runtime.containerGroups[0].containers[0].resourceAllocation.memory", Msg: "is not a valid size"},
			})
		})
	}
}

// A malformed credential reference is a manifest error like any other, and it
// arrives with its line rather than as a mystery value at deploy time.
func TestValidate_CredentialShorthandSyntax(t *testing.T) {
	err := validateString(t, "", `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
            environmentVars:
              - name: OPENAI_API_KEY
                value: dr-credential:68f0cccc0000000000000003
`)

	requireFindings(t, err, []FieldError{
		{Line: 15, Path: "artifact.spec.containerGroups[0].containers[0].environmentVars[0].value", Msg: "must be dr-credential:"},
	})
}

// Every problem in one pass, ordered by line, so one run of the command is
// enough to fix the file.
func TestValidate_ReportsEveryFindingInLineOrder(t *testing.T) {
	err := validateString(t, "", `artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 80
            imageUri: nginx:latest
runtime:
  containerGroups:
    - name: dfault
      replicaCount: 1
`)

	requireFindings(t, err, []FieldError{
		{Line: 1, Path: keyName, Msg: "is required"},
		{Line: 10, Path: "artifact.spec.containerGroups[0].containers[0].port", Msg: "must listen on 1024 or above"},
		{Line: 14, Path: "runtime.containerGroups[0].name", Msg: "matches no artifact container group"},
	})
}

// serviceWithContainerBody builds a one-container service whose image source
// lines are the caller's, so an image-source case reads as just that case.
// The container body starts at line 12.
func serviceWithContainerBody(body string) string {
	return `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
` + body
}

// runtimeWithGroupBody builds a valid service plus one runtime group whose
// body is the caller's. The group body starts at line 16.
func runtimeWithGroupBody(body string) string {
	return `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
runtime:
  containerGroups:
    - name: default
` + body
}

// Memory is what people type, not only what the file should say: case and a
// stray space are accepted and normalized, while binary units stay rejected
// because the platform would read them as their decimal namesakes.
func TestValidMemory_AcceptsWhatPeopleType(t *testing.T) {
	accepted := map[string]string{
		"512MB":   "512MB",
		"512mb":   "512MB",
		"512Mb":   "512MB",
		"512 MB":  "512MB",
		" 4gb ":   "4GB",
		"1000":    "1000",
		"2tb":     "2TB",
		"1048576": "1048576",
	}

	for input, want := range accepted {
		t.Run("accepts "+input, func(t *testing.T) {
			assert.True(t, ValidMemory(input))
			assert.Equal(t, want, NormalizeMemory(input))
		})
	}

	for _, input := range []string{"4Gi", "512Mi", "1KiB", "big", "0.5GB", "MB", "512 M B"} {
		t.Run("rejects "+input, func(t *testing.T) {
			assert.False(t, ValidMemory(input))
			assert.Equal(t, input, NormalizeMemory(input), "an unrecognized size is returned untouched")
		})
	}
}
