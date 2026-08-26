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

package del

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_ArgIsOptional(t *testing.T) {
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

// In tests stdin is not a terminal, so without --yes the command must stop
// with the confirmation-required guidance before any network call.
func TestCmd_RequiresYesWhenNonInteractive(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Contains(t, err.Error(), "--yes")
}

// Bypass works via --yes or DATAROBOT_CLI_NON_INTERACTIVE; anything else
// requires confirmation (stdin is not a terminal under go test).
func TestConfirmDelete_YesSources(t *testing.T) {
	cases := []struct {
		name          string
		env           string
		setYesFlag    bool
		wantConfirmed bool
		wantErr       bool
	}{
		{name: "env 1 bypasses prompt", env: "1", wantConfirmed: true},
		{name: "env true bypasses prompt", env: "true", wantConfirmed: true},
		{name: "empty env requires confirmation", env: "", wantErr: true},
		{name: "env false requires confirmation", env: "false", wantErr: true},
		{name: "yes flag bypasses prompt", env: "", setYesFlag: true, wantConfirmed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(reader.NonInteractiveEnv, c.env)

			cmd := Cmd()

			if c.setYesFlag {
				require.NoError(t, cmd.Flags().Set("yes", "true"))
			}

			confirmed, err := confirmDelete(cmd, "wl-1")

			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "confirmation required")
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, c.wantConfirmed, confirmed)
		})
	}
}

const boundManifest = `workloadId: 68b0c1d2e3f4a5b6c7d8e9f0
name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
`

func writeManifest(t *testing.T, dir, body string) string {
	t.Helper()

	path := filepath.Join(dir, manifest.FileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	return path
}

// The reported bug: the id the CLI wrote is the CLI's to take back, so the
// next deploy is not pointed at something that no longer exists.
func TestClearStaleBinding_ClearsAMatchingID(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Empty(t, parsed.WorkloadID())
	assert.Contains(t, out, "Removed workloadId")
}

// delete is addressed by id and can be run from any directory. A manifest
// about some other workload is none of its business, and editing it would be
// the CLI touching a file it was never asked about.
func TestClearStaleBinding_LeavesAnotherWorkloadsManifestAlone(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0ffffffffffffffffffff")

	out := buf.String()

	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", parsed.WorkloadID())
	assert.Empty(t, out)
}

// The workload is already gone by the time this runs, so nothing here may
// turn a successful deletion into a failure.
func TestClearStaleBinding_SilentWithNoManifest(t *testing.T) {
	t.Chdir(t.TempDir())

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	assert.Empty(t, out)
}

// An unparseable manifest is stepped over rather than reported: the id cannot
// be compared, so there is nothing to say about a file that may well be about
// a different workload entirely.
func TestClearStaleBinding_SilentWhenTheManifestCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "name: [unclosed\n")
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	assert.Empty(t, out)
}

// Deleting a workload leaves its artifact alone, so the local link stays
// accurate and the next deploy reuses it. That is only surprising when it is
// invisible, which is what this line exists to prevent. The remedy it names is
// the state directory, which is what `up` itself tells the user to remove when
// a locked artifact blocks a deploy; deleting the artifact instead is refused
// while it is locked and dead-ends the next deploy while it is not.
func TestClearStaleBinding_NamesTheStillLinkedArtifact(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68b0aaaa0000000000000001"}))
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	assert.Contains(t, out, "68b0aaaa0000000000000001")
	assert.Contains(t, out, "was not deleted with the workload")
	assert.Contains(t, out, wapi.Dir(dir), "the remedy has to name the directory to remove")
	assert.NotContains(t, out, "dr artifact delete", "advice that dead-ends is what this ticket is fixing")
}

// A project that never linked to an artifact has nothing to say about one.
func TestClearStaleBinding_NoArtifactNoteWithoutAStateDir(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	assert.Contains(t, out, "Removed workloadId")
	assert.NotContains(t, out, "still linked to artifact")
}

// The reporter's own reproduction deploys with --dir site and then deletes
// from the repository root. Locate walks upward, so a manifest in a
// subdirectory is invisible from there and the binding it holds would survive
// the cleanup that exists to remove it.
func TestClearStaleBinding_FindsAManifestUnderDir(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "site")
	require.NoError(t, os.Mkdir(project, 0o750))

	path := writeManifest(t, project, boundManifest)
	t.Chdir(root)

	var buf bytes.Buffer

	clearStaleBinding(&buf, "site", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Empty(t, parsed.WorkloadID())
	assert.Contains(t, out, "Removed workloadId")
}

// The workload is gone by the time the manifest is touched, so a write that
// fails must not fail the command. It does have to say so, because the user is
// the one who has to finish the job.
func TestClearStaleBinding_WarnsWhenTheManifestCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not block rename the same way on Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}

	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	// The write is atomic, so it needs to create a temporary file alongside
	// the target; a directory that refuses new entries is what breaks it.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	out := buf.String()

	assert.Contains(t, out, "Could not remove workloadId")

	// The binding is still there, so the message is telling the truth.
	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", parsed.WorkloadID())
}

// A --dir that is not a directory would otherwise be walked straight past, and
// Locate would edit an ancestor's manifest instead of the one the flag named.
// The wording covers both shapes it can take: absent, and a path that exists
// but is a file.
func TestClearStaleBinding_NamesADirThatIsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, "sight", "68b0c1d2e3f4a5b6c7d8e9f0")

	assert.Contains(t, buf.String(), "No manifest was checked")
	assert.Contains(t, buf.String(), manifest.ErrNotADirectory.Error())
	assert.Contains(t, buf.String(), "sight", "the path that was refused")

	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", parsed.WorkloadID(),
		"a typo must not fall back to editing whatever manifest is nearby")
}

// A manifest that cannot be walked safely is stepped over in silence, the same
// as one that cannot be parsed: nothing can say whether it is even about the
// workload just deleted, and `up` reports such a file in its own words.
func TestClearStaleBinding_SilentOnAnUnwalkableManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a: &a [1,1]\nb: &b [*a,*a]\nc: &c [*b,*b]\nworkloadId: *c\n")
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, ".", "68b0c1d2e3f4a5b6c7d8e9f0")

	assert.Empty(t, buf.String())
}

// The manifest path itself is the likeliest typo for --dir, and it exists, so
// a check that only asked whether the path was there would walk past it.
func TestClearStaleBinding_RejectsAFileAsDir(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)
	t.Chdir(dir)

	var buf bytes.Buffer

	clearStaleBinding(&buf, path, "68b0c1d2e3f4a5b6c7d8e9f0")

	assert.Contains(t, buf.String(), manifest.ErrNotADirectory.Error())

	parsed, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", parsed.WorkloadID())
}

// The flag is registered here and read by name in RunE, so nothing in the
// package fails if it is renamed or dropped: GetString would return "" and the
// command would quietly go back to searching upward from the working
// directory, which is the behaviour --dir exists to replace.
func TestCmd_RegistersDirFlag(t *testing.T) {
	flag := Cmd().Flags().Lookup("dir")

	require.NotNil(t, flag, "RunE reads --dir by name; renaming it here fails nothing else")
	assert.Equal(t, "string", flag.Value.Type())
}
