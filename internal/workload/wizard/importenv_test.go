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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importing is a headless run that asks for the one edit setup makes to a
// manifest already on disk.
func importing(dir string, answers Answers) Options {
	opts := headless(dir, answers)
	opts.ImportEnv = true

	return opts
}

// configured is a project that has been through setup once, with the .env it
// was set up from. The tests then grow that .env and ask what the second run
// makes of it.
func configured(t *testing.T, env string) string {
	t.Helper()

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, env)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	return dir
}

// The whole point of the flag: a variable added to .env after setup reaches
// the manifest, and the run says which one.
func TestImportEnv_AddsWhatTheManifestDoesNotDeclare(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, 1, result.EnvKeysListed)

	after := readManifest(t, dir)
	assert.Contains(t, after, "REGION")
	assert.Contains(t, after, "eu-west-1")

	// The variable that was already there is untouched, not re-added.
	assert.Equal(t, 1, strings.Count(after, "LOG_LEVEL"))
}

// The manifest is the deployed truth and .env is a local copy allowed to
// drift, so a value that changed locally does not overwrite the one deployed.
func TestImportEnv_LeavesDeclaredNamesAlone(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\n")

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, 0, result.EnvKeysListed)

	after := readManifest(t, dir)
	assert.Contains(t, after, "debug")
	assert.NotContains(t, after, "trace")
}

// Removing a line from the developer's own copy says nothing about what the
// workload should run, so nothing is taken away. .env both loses and gains a
// name here, so the import actually runs rather than returning early.
func TestImportEnv_DoesNotRemoveWhatEnvDropped(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\nREGION=eu-west-1\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nZONE=eu-west-1a\n")

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	require.Equal(t, ActionUpdated, result.Action)

	after := readManifest(t, dir)
	assert.Contains(t, after, "ZONE")
	assert.Contains(t, after, "REGION")
}

// The file belongs to the user. An edit that dropped their comments or
// reordered their keys would be a worse trade than the hand edit it replaces.
func TestImportEnv_PreservesCommentsAndHandTunedKeys(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")

	path := manifest.Path(dir)
	original := readManifest(t, dir)

	// A key the wizard never asks about, and a comment explaining it: exactly
	// what the delete-and-recreate workaround used to lose.
	edited := strings.Replace(original, "runtime:", "# tuned by hand after a load test\nruntime:", 1)
	require.NotEqual(t, original, edited)
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o600))

	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	_, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	after := readManifest(t, dir)
	assert.Contains(t, after, "# tuned by hand after a load test")
	assert.Contains(t, after, "REGION")
}

// A dry run promises to change nothing, and that has to include the credential
// store: a secret sent there is not something a preview can take back.
func TestImportEnv_DryRunWritesNothingAndStoresNothing(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	stored := credentialStore(t)

	opts := importing(dir, Answers{})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionPlanned, result.Action)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Empty(t, *stored)

	// The preview is the content the real edit would have written.
	assert.Contains(t, string(result.Content), "STRIPE_API_KEY")
}

// A secret goes to the credential store and the manifest gets the reference,
// which is the half of this the user could not do by hand.
func TestImportEnv_StoresSecretsAndReferencesThem(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	stored := credentialStore(t)

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, 0, result.EnvSecretsPending)

	require.Len(t, *stored, 1)
	assert.Equal(t, "fixture-not-a-real-key-1a2b3c4d", (*stored)[0].Value)

	after := readManifest(t, dir)
	assert.Contains(t, after, manifest.CredentialShorthandPrefix)
	// The value itself never reaches a file meant to be committed.
	assert.NotContains(t, after, "fixture-not-a-real-key-1a2b3c4d")
}

// A store that cannot be reached leaves the entry visibly unfinished rather
// than dropping the variable, which is what a deploy then refuses by name.
func TestImportEnv_UnreachableStoreLeavesThePlaceholder(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	noCredentialStore(t, errors.New("platform unreachable"))

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvSecretsPending)
	assert.Contains(t, readManifest(t, dir), manifest.CredentialPlaceholder)
}

// A manifest naming an artifact by id has no container spec of its own, so
// there is nowhere to put the variable. Saying so beats writing a key the
// deploy never reads.
func TestImportEnv_RefusesAManifestWithNoContainerSpec(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "REGION=eu-west-1\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir),
		[]byte("name: by-id\nartifactId: 68b0bbbb0000000000000002\n"), 0o600))

	_, err := Run(importing(dir, Answers{}))
	require.Error(t, err)

	assert.ErrorIs(t, err, manifest.ErrNoPrimaryContainer)
}

// Without the flag nothing changes, which is the guard the deploy path relies
// on. The drift is still named, because it is invisible until a container
// fails at first use.
func TestRun_ExistingManifestNamesTheEnvDrift(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, before, readManifest(t, dir))

	assert.Contains(t, stderr.String(), "REGION")
	assert.Contains(t, stderr.String(), "--import-env")
}

// Values are what the plan output redacts on purpose, and a warning is not the
// place that undoes it.
func TestRun_EnvDriftNoticeNamesNoValues(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "STRIPE_API_KEY")
	assert.NotContains(t, stderr.String(), "fixture-not-a-real-key-1a2b3c4d")
}

// The ordinary CI case: no .env, so nothing to compare and nothing to say.
func TestRun_NoEnvFileIsSilent(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	require.NoError(t, os.Remove(filepath.Join(dir, EnvFileName)))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.NotContains(t, stderr.String(), "--import-env")
}

// --skip-env says the file is not to be read, and the notice is reading it.
func TestRun_SkipEnvSilencesTheDriftNotice(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{SkipEnv: true})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.NotContains(t, stderr.String(), "--import-env")
}

// readManifest is the file as it stands, which is what every assertion here
// is really about.
func readManifest(t *testing.T, dir string) string {
	t.Helper()

	data, err := os.ReadFile(manifest.Path(dir))
	require.NoError(t, err)

	return string(data)
}

// A credential is created on the tenant for good, so a refusal that was
// knowable from the file must come first. Otherwise a failed run leaves a
// secret nothing references, whose name every retry then collides with.
func TestImportEnv_StoresNothingWhenTheManifestCannotTakeIt(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "STRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir),
		[]byte("name: by-id\nartifactId: 68b0bbbb0000000000000002\n"), 0o600))

	stored := credentialStore(t)

	_, err := Run(importing(dir, Answers{}))
	require.ErrorIs(t, err, manifest.ErrNoPrimaryContainer)

	assert.Empty(t, *stored)
}

// The variables the run added, so the command can name the ones whose values
// it wrote out in the clear.
func TestImportEnv_ReportsWhatItAddedForDisclosure(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\nZONE=eu-west-1a\n")

	result, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	names := make([]string, 0, len(result.Draft.EnvVars))
	for _, v := range result.Draft.EnvVars {
		names = append(names, v.Name)
	}

	assert.Equal(t, []string{"REGION", "ZONE"}, names)
	assert.Equal(t, len(result.Draft.EnvVars), result.EnvKeysListed)
}

// A .env that could not be read is not a .env with nothing new in it, and
// reporting it as success would deploy a container missing its configuration.
func TestImportEnv_RefusesAnUnparseableEnvFile(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nBAD=\"unterminated\n")

	_, err := Run(importing(dir, Answers{}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), EnvFileName)
}

// The same file, without the flag: the parse failure is named rather than
// passing as "no drift".
func TestRun_ExistingManifestNamesAnUnreadableEnvFile(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nBAD=\"unterminated\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "cannot parse")
}

// An entry the CLI wrote and never finished is skipped by every later import,
// because the name counts as declared. Saying so is what keeps the retry from
// reading as "everything is fine".
func TestImportEnv_NamesAnUnfinishedCredential(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	noCredentialStore(t, errors.New("platform unreachable"))

	first, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)
	require.Equal(t, 1, first.EnvSecretsPending)

	// The store is back, and the obvious thing to do is run it again.
	credentialStore(t)

	stderr := &bytes.Buffer{}
	opts := importing(dir, Answers{})
	opts.Stderr = stderr

	again, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, again.Action)
	assert.Contains(t, stderr.String(), "STRIPE_API_KEY")
	assert.Contains(t, stderr.String(), manifest.CredentialPlaceholder)
}

// A local-only verdict is a heuristic, and this path has no table to overrule
// it on, so the name has to be said or the user cannot learn it was held back.
func TestImportEnv_NamesWhatWasHeldBackAsLocalOnly(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nDATABASE_URL=postgres://localhost:5432/dev\n")

	stderr := &bytes.Buffer{}
	opts := importing(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr.String(), "DATABASE_URL")
	assert.Contains(t, stderr.String(), "local-only")
	// Names, never values.
	assert.NotContains(t, stderr.String(), "postgres://localhost:5432/dev")
}

// No .env is not "the manifest already declares everything".
func TestImportEnv_SaysWhenThereIsNoEnvFile(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	require.NoError(t, os.Remove(filepath.Join(dir, EnvFileName)))

	stderr := &bytes.Buffer{}
	opts := importing(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr.String(), "nothing to import")
	assert.NotContains(t, stderr.String(), "already declares")
}

// The flag names a file to add to; without one the run stops instead of
// configuring a directory that was supposedly already configured.
func TestImportEnv_RefusesWhenThereIsNoManifest(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "REGION=eu-west-1\n")

	_, err := Run(importing(dir, Answers{}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "nothing to import")
	assert.NoFileExists(t, manifest.Path(dir))
}

// Advertising a flag that cannot run on this file would be an errand. The
// notice says what the shape is instead.
func TestRun_DriftNoticeDoesNotAdvertiseAnImportThatWouldRefuse(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "REGION=eu-west-1\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir),
		[]byte("name: by-id\nartifactId: 68b0bbbb0000000000000002\n"), 0o600))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "cannot be added automatically")
	assert.NotContains(t, stderr.String(), "--import-env")
}

// A JSON run stays quiet because the command hands over no writer at all,
// rather than because each reporter remembers to check. The command test
// covers the wiring; this pins the wizard's half of it.
func TestRun_SilentWithNoWriter(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	opts := headless(dir, Answers{})
	opts.Stderr = nil

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
}

// Under JSON there is no stderr to warn on, so a .env that could not be read
// is an error rather than a silence a pipeline would read as "no variables".
func TestRun_UnreadableEnvFileIsFatalUnderJSON(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nBAD=\"unterminated\n")

	opts := headless(dir, Answers{})
	opts.Stderr = nil
	opts.JSONOutput = true

	_, err := Run(opts)
	require.Error(t, err)

	assert.Contains(t, err.Error(), EnvFileName)
}

// Past a handful the names stop being a summary, the way the neighbouring
// listings already decided.
func TestRun_DriftNoticeCapsTheList(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")

	var env strings.Builder

	env.WriteString("LOG_LEVEL=debug\n")

	for i := range 20 {
		fmt.Fprintf(&env, "SETTING_%02d=value\n", i)
	}

	writeEnvFile(t, dir, env.String())

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "and 12 more")
	assert.NotContains(t, stderr.String(), "SETTING_19")
}

// A dry run must not report the state of not having done the work: nothing
// was stored, so every secret still carries a placeholder.
func TestImportEnv_DryRunDoesNotReportPendingSecrets(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	credentialStore(t)

	opts := importing(dir, Answers{})
	opts.DryRun = true

	planned, err := Run(opts)
	require.NoError(t, err)

	actual, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, planned.EnvKeysListed, actual.EnvKeysListed)
	assert.Equal(t, actual.EnvSecretsPending, planned.EnvSecretsPending)
}

// A deploy from a subdirectory reads the manifest above it, so a run told
// there is no manifest here has been told something a deploy would contradict.
func TestImportEnv_NamesTheManifestAboveTheDirectory(t *testing.T) {
	root := configured(t, "LOG_LEVEL=debug\n")

	sub := filepath.Join(root, "service")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	writeEnvFile(t, sub, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	_, err := Run(importing(sub, Answers{}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), manifest.Path(root))
	assert.Contains(t, err.Error(), "--dir "+root)
}
