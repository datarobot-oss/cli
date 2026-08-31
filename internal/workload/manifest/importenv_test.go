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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noEnvManifest is a hand-written file with no environmentVars at all, which
// is what setup writes for a project whose .env only appeared later.
const noEnvManifest = `# Deploy config for my-app. Edit freely.
name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
`

// withEnvManifest already carries one variable, so an import has something to
// skip as well as something to add.
const withEnvManifest = `name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars:
              - name: LOG_LEVEL
                value: debug
`

// The key has to be created when the container has none, and the banner
// comment has to survive the edit.
func TestImportEnvVars_CreatesTheKeyAndKeepsComments(t *testing.T) {
	path := writeManifest(t, t.TempDir(), noEnvManifest)

	added, content, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"REGION"}, names(added))

	on := readFile(t, path)
	assert.Equal(t, string(content), on)
	assert.Contains(t, on, "# Deploy config for my-app. Edit freely.")
	assert.Contains(t, on, "environmentVars:")
	assert.Contains(t, on, "REGION")

	// The result still parses and validates, which is the only claim that
	// matters to the deploy that reads it next.
	parsed, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// A name the file already declares keeps whatever it was given.
func TestImportEnvVars_SkipsDeclaredNames(t *testing.T) {
	path := writeManifest(t, t.TempDir(), withEnvManifest)

	added, _, err := ImportEnvVars(path, []EnvVar{
		{Name: "LOG_LEVEL", Value: "trace"},
		{Name: "REGION", Value: "eu-west-1"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"REGION"}, names(added))

	on := readFile(t, path)
	assert.Contains(t, on, "debug")
	assert.NotContains(t, on, "trace")
}

// A .env is a text file and may repeat a name. The first wins, and the second
// must not land as a duplicate entry the reader would silently ignore.
func TestImportEnvVars_DoesNotDuplicateWithinOneCall(t *testing.T) {
	path := writeManifest(t, t.TempDir(), noEnvManifest)

	added, _, err := ImportEnvVars(path, []EnvVar{
		{Name: "REGION", Value: "eu-west-1"},
		{Name: "REGION", Value: "us-east-1"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"REGION"}, names(added))
	assert.Equal(t, 1, strings.Count(readFile(t, path), "REGION"))
}

// Nothing to add is not an edit: the file is left byte-for-byte alone rather
// than rewritten to the same meaning with different formatting.
func TestImportEnvVars_NothingToAddLeavesTheFileAlone(t *testing.T) {
	path := writeManifest(t, t.TempDir(), withEnvManifest)

	added, content, err := ImportEnvVars(path, []EnvVar{{Name: "LOG_LEVEL", Value: "trace"}}, false)
	require.NoError(t, err)

	assert.Empty(t, added)
	assert.Empty(t, content)
	assert.Equal(t, withEnvManifest, readFile(t, path))
}

// preview renders the edit without performing it, so a dry run shows the
// bytes the real call would have written.
func TestImportEnvVars_PreviewWritesNothing(t *testing.T) {
	path := writeManifest(t, t.TempDir(), withEnvManifest)

	added, content, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, true)
	require.NoError(t, err)

	assert.Equal(t, []string{"REGION"}, names(added))
	assert.Contains(t, string(content), "REGION")
	assert.Equal(t, withEnvManifest, readFile(t, path))
}

// A secret is written as a reference, and as the placeholder when nothing was
// stored for it, matching what the renderer emits for a fresh manifest.
func TestImportEnvVars_WritesCredentialReferences(t *testing.T) {
	path := writeManifest(t, t.TempDir(), noEnvManifest)

	_, _, err := ImportEnvVars(path, []EnvVar{
		{Name: "STORED", Secret: true, CredentialID: "66f0c1d2e3f4a5b6c7d8e9f0"},
		{Name: "PENDING", Secret: true},
	}, false)
	require.NoError(t, err)

	on := readFile(t, path)
	assert.Contains(t, on, CredentialShorthandPrefix+"66f0c1d2e3f4a5b6c7d8e9f0/")
	assert.Contains(t, on, CredentialPlaceholder)
}

// A file naming an artifact by id has no spec to add to, and the refusal says
// what to do instead.
func TestImportEnvVars_RefusesAFileWithNoSpec(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "name: by-id\nartifactId: 68b0bbbb0000000000000002\n")

	_, _, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
	require.ErrorIs(t, err, ErrNoPrimaryContainer)

	assert.Equal(t, "name: by-id\nartifactId: 68b0bbbb0000000000000002\n", readFile(t, path))
}

// EnvVarNames is what decides whether an entry in .env is new, so it has to
// read the primary container and answer emptily for a file with none.
func TestEnvVarNames(t *testing.T) {
	parsed, err := Parse([]byte(withEnvManifest), "")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"LOG_LEVEL": true}, parsed.EnvVarNames())

	none, err := Parse([]byte("name: by-id\nartifactId: 68b0\n"), "")
	require.NoError(t, err)
	assert.Empty(t, none.EnvVarNames())
}

// names is what the assertions here are about: which variables reached the
// file, in order.
func names(vars []EnvVar) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Name)
	}

	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// container is withEnvManifest's primary container with its environmentVars
// replaced, so each shape below differs from the working one in one line.
func container(envVars string) string {
	return `name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
` + envVars
}

// A key left holding nothing is the list emptied by hand, so the import fills
// it in. Appending to the null node instead would be dropped by the encoder,
// and the run would report variables it did not write.
func TestImportEnvVars_FillsAnEmptiedList(t *testing.T) {
	path := writeManifest(t, t.TempDir(), container("            environmentVars:\n"))

	added, _, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"REGION"}, names(added))
	assert.Contains(t, readFile(t, path), "REGION")

	parsed, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"REGION": true}, parsed.EnvVarNames())
}

// Everything this edit cannot add to and leave the rest of the file meaning
// what it did. Each is refused with the file untouched.
func TestImportEnvVars_RefusesShapesItCannotAppendTo(t *testing.T) {
	shared := `x-shared: &sharedEnv
  - name: SHARED
    value: from-anchor
`

	tests := map[string]struct {
		source string
		want   error
	}{
		"alias into a shared block": {
			source: shared + container("            environmentVars: *sharedEnv\n"),
			want:   ErrSharedEnvVars,
		},
		"container inheriting through a merge key": {
			source: "x-defaults: &d\n  environmentVars:\n    - name: SHARED\n      value: from-defaults\n" +
				container("            <<: *d\n"),
			want: ErrSharedEnvVars,
		},
		"a mapping instead of a list": {
			source: container("            environmentVars: {}\n"),
			want:   ErrUnreadableEnvVars,
		},
		"a scalar instead of a list": {
			source: container("            environmentVars: nope\n"),
			want:   ErrUnreadableEnvVars,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), test.source)

			_, _, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
			require.ErrorIs(t, err, test.want)

			assert.Equal(t, test.source, readFile(t, path))

			// The same verdict has to be reachable before anything is stored.
			parsed, err := Load(path)
			require.NoError(t, err)
			assert.ErrorIs(t, parsed.CanDeclareEnvVars(), test.want)
		})
	}
}

// CanDeclareEnvVars is the question a caller asks before minting a credential,
// so it has to say yes for the shapes the import can actually write.
func TestCanDeclareEnvVars_AcceptsWhatImportAccepts(t *testing.T) {
	for name, source := range map[string]string{
		"an existing list": withEnvManifest,
		"no key at all":    noEnvManifest,
		"an emptied list":  container("            environmentVars:\n"),
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := Parse([]byte(source), "")
			require.NoError(t, err)
			assert.NoError(t, parsed.CanDeclareEnvVars())
		})
	}
}

// An entry the CLI wrote and never finished stays findable, because the name
// counts as declared and every later import skips it.
func TestPendingEnvNames(t *testing.T) {
	parsed, err := Parse([]byte(container(
		"            environmentVars:\n"+
			"              - name: LOG_LEVEL\n                value: debug\n"+
			"              - name: STRIPE_API_KEY\n"+
			"                value: "+CredentialShorthandPrefix+CredentialPlaceholder+"/apiToken\n")), "")
	require.NoError(t, err)

	assert.Equal(t, []string{"STRIPE_API_KEY"}, parsed.PendingEnvNames())
}

// A colon-separated value is a string to yaml.v3 and a base-60 number to the
// older parsers that also read this file, so it has to be written quoted.
func TestImportEnvVars_QuotesSexagesimalValues(t *testing.T) {
	path := writeManifest(t, t.TempDir(), noEnvManifest)

	_, _, err := ImportEnvVars(path, []EnvVar{{Name: "CRON_WINDOW", Value: "12:30"}}, false)
	require.NoError(t, err)

	assert.Contains(t, readFile(t, path), `"12:30"`)
}

// The anchor side of a shared block, which is the half a check on aliases
// alone misses: the primary defines it, somebody else reads it.
func TestImportEnvVars_RefusesTheAnchorSideOfASharedBlock(t *testing.T) {
	source := container(`            environmentVars: &sharedEnv
              - name: LOG_LEVEL
                value: debug
`) + `x-sidecar:
  environmentVars: *sharedEnv
`

	path := writeManifest(t, t.TempDir(), source)

	_, _, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
	require.ErrorIs(t, err, ErrSharedEnvVars)

	assert.Equal(t, source, readFile(t, path))

	parsed, err := Load(path)
	require.NoError(t, err)
	assert.ErrorIs(t, parsed.CanDeclareEnvVars(), ErrSharedEnvVars)
}

// Sharing anywhere on the walk counts, not only at the last step: an anchored
// container or an aliased spec is a node whose edit lands somewhere the user
// is not looking.
func TestImportEnvVars_RefusesSharingAnywhereOnTheWalk(t *testing.T) {
	tests := map[string]string{
		"an anchored container": `name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - &base
            name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
      - name: second
        containers:
          - *base
`,
		"an aliased spec": `x-shared: &commonSpec
  containerGroups:
    - name: default
      containers:
        - name: primary
          primary: true
          port: 8080
          imageUri: registry/team/app:v1
name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec: *commonSpec
`,
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), source)

			_, _, err := ImportEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
			require.ErrorIs(t, err, ErrSharedEnvVars)

			assert.Equal(t, source, readFile(t, path))
		})
	}
}

// A name the container inherits is a name it serves, so reading the file as
// declaring nothing and offering to add them all would delete the inheritance.
func TestEnvVarNames_RefusesRatherThanMisreadAMergeKey(t *testing.T) {
	source := "x-defaults: &d\n  environmentVars:\n    - name: SHARED\n      value: from-defaults\n" +
		container("            <<: *d\n")

	parsed, err := Parse([]byte(source), "")
	require.NoError(t, err)

	require.ErrorIs(t, parsed.CanDeclareEnvVars(), ErrSharedEnvVars)
	assert.Empty(t, parsed.EnvVarNames())
}

// The unfinished-credential report reads the same references the deploy
// refuses, so a literal that merely contains the word is not one of them.
func TestPendingEnvNames_OnlyCountsCredentialReferences(t *testing.T) {
	parsed, err := Parse([]byte(container(
		"            environmentVars:\n"+
			"              - name: MOTD\n                value: replace PLACEHOLDER with your name\n"+
			"              - name: STRIPE_API_KEY\n"+
			"                value: "+CredentialShorthandPrefix+CredentialPlaceholder+"/apiToken\n")), "")
	require.NoError(t, err)

	assert.Equal(t, []string{"STRIPE_API_KEY"}, parsed.PendingEnvNames())
}

// The update half owes the same refusal as the import half: an entry another
// container reads must not be rewritten from here.
func TestUpdateEnvVars_RefusesASharedEntry(t *testing.T) {
	source := container(`            environmentVars:
              - &shared
                name: LOG_LEVEL
                value: info
`) + `x-sidecar:
  environmentVars:
    - *shared
`

	path := writeManifest(t, t.TempDir(), source)

	_, _, err := UpdateEnvVars(path, []EnvVar{{Name: "LOG_LEVEL", Value: "trace"}}, false)
	require.ErrorIs(t, err, ErrSharedEnvVars)

	assert.Equal(t, source, readFile(t, path))
}

// An anchored value is read by name elsewhere, so replacing it would change
// what that name means.
func TestUpdateEnvVars_RefusesAnAnchoredValue(t *testing.T) {
	source := container(`            environmentVars:
              - name: LOG_LEVEL
                value: &lvl info
`)

	path := writeManifest(t, t.TempDir(), source)

	_, _, err := UpdateEnvVars(path, []EnvVar{{Name: "LOG_LEVEL", Value: "trace"}}, false)
	require.ErrorIs(t, err, ErrSharedEnvVars)

	assert.Equal(t, source, readFile(t, path))
}

// The value is edited rather than replaced, so everything the user wrote
// around it survives, and a value another parser would misread is quoted.
func TestUpdateEnvVars_KeepsCommentsAndQuotesWhatNeedsIt(t *testing.T) {
	source := container(`            environmentVars:
              - name: LOG_LEVEL
                value: info # staging only
              - name: CRON_WINDOW
                value: "09:00"
`)

	path := writeManifest(t, t.TempDir(), source)

	changed, _, err := UpdateEnvVars(path, []EnvVar{
		{Name: "LOG_LEVEL", Value: "trace"},
		{Name: "CRON_WINDOW", Value: "12:30"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"LOG_LEVEL", "CRON_WINDOW"}, names(changed))

	on := readFile(t, path)
	assert.Contains(t, on, "# staging only")
	assert.Contains(t, on, `"12:30"`)
}

// A credential-backed entry has no literal to rewrite: its value lives in the
// store, and the id in the file stays the id in the file.
func TestUpdateEnvVars_LeavesCredentialReferencesAlone(t *testing.T) {
	source := container(`            environmentVars:
              - name: STRIPE_API_KEY
                value: ` + CredentialShorthandPrefix + `66f0c1d2e3f4a5b6c7d8e9f0/apiToken
`)

	path := writeManifest(t, t.TempDir(), source)

	changed, _, err := UpdateEnvVars(path, []EnvVar{{Name: "STRIPE_API_KEY", Value: "fixture-key-new-e5e5"}}, false)
	require.NoError(t, err)

	assert.Empty(t, changed)
	assert.Equal(t, source, readFile(t, path))
}

// DeclaredEnvVars carries which field of a credential an entry reads, because
// only the one this CLI writes is one an update may re-send to.
func TestDeclaredEnvVars_CarriesTheCredentialField(t *testing.T) {
	parsed, err := Parse([]byte(container(`            environmentVars:
              - name: LOG_LEVEL
                value: debug
              - name: API_KEY
                value: `+CredentialShorthandPrefix+`66f0c1d2e3f4a5b6c7d8e9f0/apiToken
              - name: DB_PASSWORD
                value: `+CredentialShorthandPrefix+`66f0c1d2e3f4a5b6c7d8e9f1/password
`)), "")
	require.NoError(t, err)

	declared := parsed.DeclaredEnvVars()
	require.Len(t, declared, 3)

	assert.False(t, declared[0].Secret())
	assert.Equal(t, "debug", declared[0].Value)
	assert.Equal(t, CredentialKeyWritten, declared[1].CredentialKey)
	assert.Equal(t, "password", declared[2].CredentialKey)
}

// A group with no containers is a group to walk past, not a crash.
func TestEditableContainer_SurvivesAGroupWithNoContainers(t *testing.T) {
	parsed, err := Parse([]byte(`name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: empty
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
`), "")
	require.NoError(t, err)

	assert.NoError(t, parsed.CanDeclareEnvVars())
}

// The read-only guard has to be read-only: a caller asks it before minting a
// credential, and anything that compiles the same tree afterwards would ship
// whatever the question left behind.
func TestCanDeclareEnvVars_ChangesNothing(t *testing.T) {
	parsed, err := Parse([]byte(noEnvManifest), "")
	require.NoError(t, err)

	require.NoError(t, parsed.CanDeclareEnvVars())

	// Asked of the tree the guard was handed, which is what a later compile or
	// re-validation would walk.
	assert.Empty(t, parsed.DeclaredEnvVars())
	assert.Empty(t, parsed.EnvVarNames())
}
