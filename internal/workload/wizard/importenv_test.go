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

// updating is a headless run that asks for the other half: the values of
// names the manifest already declares.
func updating(dir string, answers Answers) Options {
	opts := headless(dir, answers)
	opts.UpdateEnv = true

	return opts
}

// A literal whose .env value changed is named, and the flag that applies it.
func TestRun_NamesAChangedLiteral(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "LOG_LEVEL")
	assert.Contains(t, stderr.String(), "--update-env")
	// Names, never values: neither side of the comparison is printed.
	assert.NotContains(t, stderr.String(), "trace")
}

// An update adds no names, so one the manifest is missing is still missing
// when it finishes. This used to be said only on the run that found nothing to
// do, which left the run that reconciled every value it could reading as a
// clean bill of health for a file short a variable.
func TestUpdateEnv_NamesUndeclaredVariablesAfterRewritingOne(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\nREGION=eu-west-1\n")

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)
	require.Equal(t, 1, result.EnvValuesUpdated, "the run has to have changed something")

	assert.Contains(t, stderr.String(), "does not declare")
	assert.Contains(t, stderr.String(), "REGION")
	assert.Contains(t, stderr.String(), "--import-env")
}

// The other half of the same branch: an import puts the missing names in the
// file, so naming them afterwards would report as missing exactly what the run
// just added.
func TestImportEnv_DoesNotNameWhatItJustAdded(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	stderr := &bytes.Buffer{}
	opts := importing(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)
	require.Equal(t, 1, result.EnvKeysListed, "the run has to have added something")

	assert.NotContains(t, stderr.String(), "does not declare")
}

// --update-env rewrites it, and leaves every other key alone.
func TestUpdateEnv_RewritesAChangedLiteral(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\nREGION=eu-west-1\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\nREGION=eu-west-1\n")

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, 1, result.EnvValuesUpdated)

	after := readManifest(t, dir)
	assert.Contains(t, after, "trace")
	assert.NotContains(t, after, "debug")
	assert.Contains(t, after, "eu-west-1")
}

// A name .env does not define is not drift, so an update leaves it alone.
func TestUpdateEnv_LeavesUndefinedNamesAlone(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\nREGION=eu-west-1\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\n")

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, readManifest(t, dir), "eu-west-1")
}

// The case the original report was about: a rotated key reaches the workload,
// through the credential the manifest already points at rather than a new one.
func TestUpdateEnv_RotatesADeclaredSecret(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	stored := credentialStore(t)
	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)
	require.Len(t, *stored, 1)

	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	sent := rotatingStore(t)

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvSecretsRotated)

	// Sent to the credential the file already names, so no second credential
	// is created and the manifest does not move.
	require.Len(t, *sent, 1)
	assert.Contains(t, before, (*sent)[0].Name)
	assert.Equal(t, "fixture-key-after-rotation-b2b2", (*sent)[0].Value)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Len(t, *stored, 1)
}

// A secret's stored value is never returned, so the notice has to say it
// cannot be checked rather than let silence read as "that one is fine".
func TestRun_SaysASecretCannotBeCompared(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err = Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "LLM_API_KEY")
	assert.Contains(t, stderr.String(), "cannot be compared")
	assert.NotContains(t, stderr.String(), "fixture-key-before-rotation-a1a1")
}

// A rotation that fails leaves the previous value serving, which nothing
// downstream can notice, so it has to be said out loud.
func TestUpdateEnv_NamesAFailedRotation(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	failingRotation(t, errors.New("platform unreachable"))

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, 0, result.EnvSecretsRotated)
	assert.Contains(t, stderr.String(), "LLM_API_KEY")
	assert.Contains(t, stderr.String(), "keeps the value it has")
}

// A dry run promises to change nothing, and a re-sent secret is not something
// a preview can take back.
func TestUpdateEnv_DryRunSendsNothing(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")
	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LOG_LEVEL=trace\nSTRIPE_API_KEY=fixture-not-a-real-key-9z8y7x6w\n")

	sent := rotatingStore(t)

	opts := updating(dir, Answers{})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionPlanned, result.Action)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Empty(t, *sent)
}

// Both flags together do both jobs in one run.
func TestEditEnv_BothFlagsAddAndUpdate(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\nREGION=eu-west-1\n")

	opts := importing(dir, Answers{})
	opts.UpdateEnv = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, 1, result.EnvKeysListed)
	assert.Equal(t, 1, result.EnvValuesUpdated)

	after := readManifest(t, dir)
	assert.Contains(t, after, "trace")
	assert.Contains(t, after, "REGION")
}

// A rotation is a write to the credential store, and the file the reference
// lives in does not move. Calling the run an update would have the command
// report an edit to a manifest that is byte for byte what it was.
func TestUpdateEnv_RotationIsNotAFileEdit(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")
	rotatingStore(t)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, 1, result.EnvSecretsRotated)
	assert.Equal(t, before, readManifest(t, dir))

	// The run did something, so the line for a run that found nothing to do
	// must not also appear.
	assert.Contains(t, stderr.String(), "Re-sent 1 secret")
	assert.NotContains(t, stderr.String(), "already gives every variable")
}

// A preview of --update-env that said nothing about the secrets would leave
// out the only part of it that reaches the tenant.
func TestUpdateEnv_DryRunSaysWhatItWouldReSend(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")
	sent := rotatingStore(t)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.DryRun = true
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionPlanned, result.Action)
	assert.Equal(t, 1, result.EnvSecretsRotated)
	assert.Empty(t, *sent)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Contains(t, stderr.String(), "would be re-sent")
}

// A failed re-send leaves the old value serving, changes no file and stops
// nothing, so a caller reading the result rather than the terminal needs a
// count of its own to tell that run from a clean one.
func TestUpdateEnv_CountsAFailedRotation(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	failingRotation(t, errors.New("platform unreachable"))

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, 0, result.EnvSecretsRotated)
	assert.Equal(t, 1, result.EnvSecretsNotRotated)

	// The failure has just been named; saying everything already matches in
	// the next breath would contradict it.
	assert.NotContains(t, stderr.String(), "already gives every variable")
}

// The two flags do opposite things, so the run that found nothing to do says
// which of the two it found nothing for.
func TestUpdateEnv_NothingToDoIsAboutValues(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr.String(), "already gives every variable it declares the value .env does")
	assert.NotContains(t, stderr.String(), "that can be imported")
}

// The values are read by the walk that resolves aliases and applied by the one
// that refuses them, so a file can show its drift and still not take the edit.
// Naming the flag there would send the user on an errand.
func TestRun_ValueDriftNamesTheRefusalNotTheFlag(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(sharedEnvManifest), 0o600))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "LOG_LEVEL")
	assert.Contains(t, stderr.String(), "cannot be edited automatically")
	assert.NotContains(t, stderr.String(), "--update-env")
}

// sharedEnvManifest is a valid manifest whose primary container shares its
// environment with another through an anchor, which is the shape every edit
// here refuses.
const sharedEnvManifest = `name: shared
artifact:
  name: shared-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars: &sharedEnv
              - name: LOG_LEVEL
                value: debug
          - name: sidecar
            imageUri: registry/team/side:v1
            environmentVars: *sharedEnv
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
        - name: sidecar
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// A value the classifier reads as local-only must never reach the tenant. It
// is held back from the file everywhere else, and a credential is the one
// place that holding back cannot be undone.
func TestUpdateEnv_NeverRotatesALocalOnlyValue(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "DATABASE_URL=postgres://user:pw@db.prod.example.com:5432/app\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)
	require.Contains(t, readManifest(t, dir), manifest.CredentialShorthandPrefix)

	// Repointed at the developer's own machine, which the classifier holds back.
	writeEnvFile(t, dir, "DATABASE_URL=postgres://localhost:5432/dev\n")

	sent := rotatingStore(t)

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, 0, result.EnvSecretsRotated)
	assert.Empty(t, *sent)
}

// --skip-env says the file is not to be read, and a rotation reads it.
func TestUpdateEnv_SkipEnvSendsNothing(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	sent := rotatingStore(t)

	_, err = Run(updating(dir, Answers{SkipEnv: true}))
	require.NoError(t, err)

	assert.Empty(t, *sent)
}

// A reference to a field this command does not write points at a credential
// other assets may share, so it is named rather than overwritten.
func TestUpdateEnv_RefusesACredentialFieldItDoesNotWrite(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "DB_PASSWORD=fixture-not-a-real-key-1a2b3c4d\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	// Hand-repointed at the password field of a shared credential.
	path := manifest.Path(dir)
	on := readManifest(t, dir)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(on, "/apiToken", "/password", 1)), 0o600))

	sent := rotatingStore(t)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Empty(t, *sent)
	assert.Equal(t, 1, result.EnvSecretsNotRotated)
	assert.Contains(t, stderr.String(), "DB_PASSWORD")
}

// The flag names a file to act on; without one the run stops rather than
// running setup and minting for the whole .env.
func TestUpdateEnv_RefusesWhenThereIsNoManifest(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "STRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	stored := credentialStore(t)

	_, err := Run(updating(dir, Answers{}))
	require.Error(t, err)

	// The verb is the one that was asked for: an --update-env run told about
	// importing is being answered about a flag it did not pass.
	assert.Contains(t, err.Error(), "nothing to update")
	assert.NoFileExists(t, manifest.Path(dir))
	assert.Empty(t, *stored)
}

// A .env that could not be read is not one with nothing to update.
func TestUpdateEnv_RefusesAnUnparseableEnvFile(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nBAD=\"unterminated\n")

	_, err := Run(updating(dir, Answers{}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), EnvFileName)
}

// A rotation writes to the credential store, not to the file, so it must not
// report the manifest as edited.
func TestUpdateEnv_RotationAloneIsNotAFileEdit(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	before := readManifest(t, dir)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")
	rotatingStore(t)

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvSecretsRotated)
	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, before, readManifest(t, dir))
}

// The preview says what it would send, because that is the half of this flag
// that cannot be undone by deleting a file.
func TestUpdateEnv_DryRunCountsWhatItWouldSend(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	sent := rotatingStore(t)

	opts := updating(dir, Answers{})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvSecretsRotated)
	assert.Empty(t, *sent)
}

// An update alone still owes the names the manifest is missing.
func TestUpdateEnv_StillNamesUndeclaredVariables(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nREGION=eu-west-1\n")

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "REGION")
	assert.NotContains(t, stderr.String(), "already declares every variable")
}

// A value that changed enough to change the classifier's verdict is acted on
// by neither half, so it has to be named.
func TestRun_NamesAReclassifiedVariable(t *testing.T) {
	dir := configured(t, "REGION=eu-west-1\n")
	require.Contains(t, readManifest(t, dir), "eu-west-1")

	writeEnvFile(t, dir, "REGION=fixture-not-a-real-key-1a2b3c4d\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "REGION")
	assert.Contains(t, stderr.String(), "different kind")
	assert.NotContains(t, stderr.String(), "fixture-not-a-real-key-1a2b3c4d")
}

// A credential id pasted by hand may name one other workloads read, and the
// store is tenant-wide. Overwriting it from a flag about this project's .env
// is the damage the ownership check refuses.
func TestUpdateEnv_RefusesACredentialThisProjectDidNotCreate(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-before-rotation-a1a1\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	// Repointed at an id the fake store does not know, the way a user pasting
	// a shared credential's id would leave it.
	path := manifest.Path(dir)
	on := readManifest(t, dir)
	require.NoError(t, os.WriteFile(path,
		[]byte(strings.Replace(on, "66f000000000000000000001", "66f0aaaaaaaaaaaaaaaaaaaa", 1)), 0o600))

	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	sent := rotatingStore(t)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Empty(t, *sent)
	assert.Equal(t, 0, result.EnvSecretsRotated)
	assert.Equal(t, 1, result.EnvSecretsNotRotated)
	assert.Contains(t, stderr.String(), "LLM_API_KEY")
}

// An entry the CLI never finished has no stored value to compare or re-send,
// so the value notice must not offer a flag that skips it.
func TestRun_DoesNotOfferToRotateAPlaceholder(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\nSTRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n")

	noCredentialStore(t, errors.New("platform unreachable"))

	_, err := Run(importing(dir, Answers{}))
	require.NoError(t, err)
	require.Contains(t, readManifest(t, dir), manifest.CredentialPlaceholder)

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err = Run(opts)
	require.NoError(t, err)

	// warnEnvHeldBack owns this one; the value notice must not contradict it.
	assert.Contains(t, stderr.String(), manifest.CredentialPlaceholder)
	assert.NotContains(t, stderr.String(), "cannot be compared")
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

// A value written as a mapping is not something either flag rewrites, so
// calling it drift would send the user to a flag that changes nothing, every
// run, for ever.
func TestRun_StructuredValueIsNotReportedAsDrift(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "SETTINGS=plain\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(structuredValueManifest), 0o600))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "SETTINGS")
	assert.Contains(t, stderr.String(), "not a plain string")
	assert.NotContains(t, stderr.String(), "Apply it with")
}

// The summary of a run that changed nothing must not claim an agreement the
// notice above it has just denied.
func TestUpdateEnv_NoOpDoesNotClaimAgreementItCannotVerify(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "SETTINGS=plain\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(structuredValueManifest), 0o600))

	before := readManifest(t, dir)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Contains(t, stderr.String(), "not a plain string")
	assert.NotContains(t, stderr.String(), "already gives every variable")
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

// A container that inherits its environment through a merge key is serving
// names the edit's walk cannot see, and EnvVarNames answers the empty set for
// exactly that shape. Read as "the manifest declares nothing", the notice
// would name every local-only variable as deliberately omitted and send the
// user off to hand-add one the inherited block may already carry.
func TestRun_DriftNoticeDoesNotGuessAtAnInheritedEnvironment(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	writeEnvFile(t, dir, "DATABASE_URL=postgres://localhost:5432/dev\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(`name: my-app
x-defaults: &d
  environmentVars:
    - name: DATABASE_URL
      value: postgres://db/prod
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
            <<: *d
`), 0o600))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.NotContains(t, stderr.String(), "local-only")
	assert.NotContains(t, stderr.String(), "DATABASE_URL")
}

// The count in the refusal is of what .env offers, not of a drift the walk
// cannot measure: saying "N the manifest does not declare" about a file whose
// declarations are unreadable states as fact the one thing that is unknown.
func TestRun_DriftRefusalDoesNotClaimTheManifestDeclaresNothing(t *testing.T) {
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
	assert.NotContains(t, stderr.String(), "does not declare")
}

// The notice a command that was asked to do something else prints about these
// two files: the same drifts setup reports, in terms of the caller's own flags
// rather than another command's.
func TestReportEnvDrift_NamesBothDriftsAndTheCallersOwnCommand(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\nREGION=eu-west-1\n")

	parsed, err := manifest.Load(manifest.Path(dir))
	require.NoError(t, err)

	stderr := &bytes.Buffer{}

	ReportEnvDrift(dir, "dr workload up", stderr, parsed)

	out := stderr.String()

	assert.Contains(t, out, "REGION", "a name .env has and the manifest does not")
	assert.Contains(t, out, "LOG_LEVEL", "a declared value .env no longer agrees with")
	assert.Contains(t, out, "'dr workload up --import-env'")
	assert.Contains(t, out, "'dr workload up --update-env'")

	assert.NotContains(t, out, "dr workload config", "the caller named its own command")
	assert.NotContains(t, out, "trace", "names, never values")
}

// Nothing to say is said with silence. A caller prints this above its own
// output, where a line meaning "the two files agree" would be noise on every
// run of every project.
func TestReportEnvDrift_SilentWhenTheFilesAgree(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\n")

	parsed, err := manifest.Load(manifest.Path(dir))
	require.NoError(t, err)

	stderr := &bytes.Buffer{}

	ReportEnvDrift(dir, "dr workload up", stderr, parsed)

	assert.Empty(t, stderr.String())
}

// The standing facts stay with the command that is about configuration. None
// of them can be settled by anything the reader of a deploy is about to run,
// so said there each would print on every run for the life of the project, and
// the drift beside them is what gets skipped along with them.
func TestReportEnvDrift_LeavesTheStandingLinesToConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(credentialEnvManifest), 0o600))
	writeEnvFile(t, dir, "LOG_LEVEL=debug\n"+
		// A credential's value, which nothing can compare with .env.
		"STRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n"+
		// A classifier verdict: held back on purpose, and no flag adds it.
		"DATABASE_URL=postgres://localhost:5432/dev\n")

	parsed, err := manifest.Load(manifest.Path(dir))
	require.NoError(t, err)

	setup := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = setup

	_, err = Run(opts)
	require.NoError(t, err)
	require.Contains(t, setup.String(), "cannot be compared",
		"setup says them, and the point of this test is the command that does not")
	require.Contains(t, setup.String(), "local-only")

	notice := &bytes.Buffer{}

	ReportEnvDrift(dir, "dr workload up", notice, parsed)

	assert.Empty(t, notice.String(), "everything that has moved agrees")
}

// A manifest whose environment is shared through a YAML anchor takes neither
// flag. That refusal counts the whole of .env as un-addable, because the walk
// that would read the declared names is the one the shape defeats, so on a
// deploy it would report as missing the names the container already carries
// through the alias, on every run, with nothing that clears it. What has
// actually moved is still said, with the refusal in place of the remedy.
func TestReportEnvDrift_DoesNotCountAllOfEnvAsMissingOnAManifestNoFlagCanEdit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(sharedEnvManifest), 0o600))
	writeEnvFile(t, dir, "LOG_LEVEL=trace\nREGION=eu-west-1\n")

	parsed, err := manifest.Load(manifest.Path(dir))
	require.NoError(t, err)
	require.Error(t, parsed.CanDeclareEnvVars(), "the fixture has to be a manifest the flags refuse")

	notice := &bytes.Buffer{}

	ReportEnvDrift(dir, "dr workload up", notice, parsed)

	out := notice.String()

	assert.NotContains(t, out, "cannot be added automatically")
	assert.NotContains(t, out, "2 variables", "LOG_LEVEL is declared, through the alias")

	assert.Contains(t, out, "LOG_LEVEL", "a value that really has moved is still drift")
	assert.Contains(t, out, "cannot be edited automatically",
		"and the refusal takes the place of a remedy that would not run")
}

// credentialEnvManifest declares one literal and one credential-backed
// variable, which is the pair a drift notice has to treat differently: one
// value can be compared with .env and the other never can.
const credentialEnvManifest = `name: my-app
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
              - name: STRIPE_API_KEY
                value: dr-credential:68f0cccc0000000000000003/apiToken
`

// structuredValueManifest declares a variable whose value is a mapping, which
// is a hand edit this CLI reads but will not overwrite.
const structuredValueManifest = `name: structured
artifact:
  name: structured-artifact
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
              - name: SETTINGS
                value:
                  nested: thing
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// A value borrowed from an anchor elsewhere in the file is drift the notice
// can read and the edit must not apply: writing that scalar would change every
// other name reading through it. Skipping it in silence was the worse half of
// the choice, because the run then reported a file it had not touched as
// already matching .env.
func TestUpdateEnv_RefusesAValueBorrowedFromAnAnchor(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(aliasedValueManifest), 0o600))

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.ErrorIs(t, err, manifest.ErrSharedEnvVars)

	assert.Equal(t, aliasedValueManifest, readManifest(t, dir))
	assert.NotContains(t, stderr.String(), "already gives every variable")
}

// aliasedValueManifest declares its environment inline, in a block nothing
// else shares, and gives one variable a value borrowed from an anchor the rest
// of the file reads too.
const aliasedValueManifest = `name: aliased
x-defaults: &level debug
artifact:
  name: aliased-artifact
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
                value: *level
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// A declared variable whose .env value has turned local-only is one the
// rotation is right to hold back and one no notice covered: the held-back line
// is about names the manifest does not carry. Unsaid, the run reported a value
// it had just declined to send as already agreeing with .env.
func TestUpdateEnv_NamesADeclaredValueTurnedLocalOnly(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "DATABASE_URL=postgres://user:pw@db.prod.example.com:5432/app\n")

	credentialStore(t)

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	// Repointed at the developer's own machine, which the classifier holds back.
	writeEnvFile(t, dir, "DATABASE_URL=postgres://localhost:5432/dev\n")

	sent := rotatingStore(t)

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	_, err = Run(opts)
	require.NoError(t, err)

	assert.Empty(t, *sent)
	assert.Contains(t, stderr.String(), "DATABASE_URL")
	assert.Contains(t, stderr.String(), "declared variable as local-only")
	assert.NotContains(t, stderr.String(), "already gives every variable")
}

// The missing-name notice ends in the held-back one, so an update that found
// nothing to do and printed both said the same name twice.
func TestUpdateEnv_HeldBackNamesAreSaidOnce(t *testing.T) {
	dir := configured(t, "LOG_LEVEL=debug\nDATABASE_URL=postgres://localhost:5432/dev\n")

	stderr := &bytes.Buffer{}
	opts := updating(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(stderr.String(), "deliberately left out of the manifest"))
}

// A container whose environment another one reads through an anchor takes no
// edit here, and a rotation is not one: it writes to the credential store and
// leaves the file byte for byte what it was. Refusing it on the file's account
// left a manifest of that shape no way to re-send a rotated key at all.
func TestUpdateEnv_RotatesThroughASharedEnvironment(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(sharedEnvSecretManifest), 0o600))

	ownedCredential(t, "shared/LLM_API_KEY")

	sent := rotatingStore(t)

	result, err := Run(updating(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, 1, result.EnvSecretsRotated)
	assert.Equal(t, sharedEnvSecretManifest, readManifest(t, dir))

	require.Len(t, *sent, 1)
	assert.Equal(t, "66f000000000000000000001", (*sent)[0].Name)
	assert.Equal(t, "fixture-key-after-rotation-b2b2", (*sent)[0].Value)
}

// The other side of that line: once a literal in the shared block is one .env
// would rewrite, the run is a write into the file after all, and the refusal
// is about exactly that.
func TestUpdateEnv_StillRefusesALiteralInASharedEnvironment(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LOG_LEVEL=trace\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(sharedEnvManifest), 0o600))

	sent := rotatingStore(t)

	_, err := Run(updating(dir, Answers{}))
	require.ErrorIs(t, err, manifest.ErrSharedEnvVars)

	assert.Empty(t, *sent)
	assert.Equal(t, sharedEnvManifest, readManifest(t, dir))
}

// sharedEnvSecretManifest shares its environment the way sharedEnvManifest
// does, and the one variable in the block is a secret: there is no literal
// here for the refusal to be about.
const sharedEnvSecretManifest = `name: shared
artifact:
  name: shared-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars: &sharedEnv
              - name: LLM_API_KEY
                value: dr-credential:66f000000000000000000001/apiToken
          - name: sidecar
            imageUri: registry/team/side:v1
            environmentVars: *sharedEnv
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
        - name: sidecar
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// A shared environment stops the rewrite, not the rotation, so the notice
// about secrets names the flag rather than the refusal: telling the user this
// manifest cannot be edited would talk them out of the one command that works
// on it.
func TestRun_SecretDriftNamesTheFlagThroughASharedEnvironment(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 8080\n")
	writeEnvFile(t, dir, "LLM_API_KEY=fixture-key-after-rotation-b2b2\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(sharedEnvSecretManifest), 0o600))

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "LLM_API_KEY")
	assert.Contains(t, stderr.String(), "re-send it with 'dr workload config --update-env'")
}
