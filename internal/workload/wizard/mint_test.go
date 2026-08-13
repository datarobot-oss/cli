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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedCredential is one call to the credential store, kept so a test can
// assert what was sent without the store existing.
type storedCredential struct {
	Name  string
	Value string
}

// credentialStore makes storing succeed, and records what was stored. Every
// test that imports a .env carrying a secret has to say which store it runs
// against, because the difference is whether the manifest ends up with an id
// or with a placeholder.
func credentialStore(t *testing.T) *[]storedCredential {
	t.Helper()

	stored := make([]storedCredential, 0)

	original := createCredentialFn
	createCredentialFn = func(name, value string) (*workload.Credential, error) {
		stored = append(stored, storedCredential{Name: name, Value: value})

		return &workload.Credential{CredentialID: "66f0000000000000000000" + itoa(len(stored)), Name: name}, nil
	}

	t.Cleanup(func() { createCredentialFn = original })

	return &stored
}

// noCredentialStore makes storing fail the way an unreachable platform does.
func noCredentialStore(t *testing.T, err error) {
	t.Helper()

	original := createCredentialFn
	createCredentialFn = func(string, string) (*workload.Credential, error) { return nil, err }

	t.Cleanup(func() { createCredentialFn = original })
}

// itoa is strconv.Itoa padded to two digits, so the ids the fake store hands
// out are the same length as real ones.
func itoa(n int) string {
	return fmt.Sprintf("%02d", n)
}

// envProject is a project with a Dockerfile and a .env holding one ordinary
// setting and one secret.
func envProject(t *testing.T) string {
	t.Helper()

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("LOG_LEVEL=debug\nOPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	return dir
}

// The point of the whole import: the value goes from .env to the credential
// store, and what lands in the committed file is an id.
func TestRun_SecretsBecomeCredentials(t *testing.T) {
	stored := credentialStore(t)
	dir := envProject(t)

	result, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	require.Len(t, *stored, 1, "only the secret is stored; an ordinary setting is a literal")
	assert.Equal(t, "my-app/OPENAI_API_KEY", (*stored)[0].Name,
		"the store is tenant-wide, so a bare variable name would collide with every other project")
	assert.Equal(t, "sk-abcdefghijklmnopqrstuvwxyz01", (*stored)[0].Value)

	written := string(result.Content)

	assert.Contains(t, written, "dr-credential:66f000000000000000000001/apiToken")
	assert.NotContains(t, written, "PLACEHOLDER")
	assert.NotContains(t, written, "sk-abcdefghijklmnopqrstuvwxyz01", "the value never reaches the file")
	assert.Equal(t, 0, result.EnvSecretsPending, "nothing is left for the user to finish")
}

// A store that cannot be reached leaves the entry as it was before this
// existed: present, obviously unfinished, and refused by name at deploy.
func TestRun_UnreachableStoreLeavesThePlaceholder(t *testing.T) {
	noCredentialStore(t, errors.New("dial tcp: connection refused"))

	dir := envProject(t)

	var stderr strings.Builder

	opts := headless(dir, Answers{Name: "my-app"})
	opts.Stderr = &stderr

	result, err := Run(opts)
	require.NoError(t, err, "one secret failing must not throw away eight answered questions")

	assert.Contains(t, string(result.Content), "dr-credential:PLACEHOLDER/apiToken")
	assert.Equal(t, 1, result.EnvSecretsPending)
	assert.Contains(t, stderr.String(), "OPENAI_API_KEY was not stored")
	assert.Contains(t, stderr.String(), "connection refused")
	assert.NotContains(t, stderr.String(), "sk-abcdefghijklmnopqrstuvwxyz01",
		"a message about a secret must not carry it")
}

// Running setup twice on the same project is the common way to meet a name
// that is taken, and reusing whatever holds it would deploy a value the user
// never supplied.
func TestRun_TakenCredentialNameSaysHowToFinish(t *testing.T) {
	noCredentialStore(t, &drapi.HTTPError{StatusCode: http.StatusConflict})

	dir := envProject(t)

	var stderr strings.Builder

	opts := headless(dir, Answers{Name: "my-app"})
	opts.Stderr = &stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "already exists")
	assert.Contains(t, stderr.String(), "pasting its id")
}

// A dry run promises to change nothing, and a credential is the one thing
// this command creates that outlives the file it was about to write.
func TestRun_DryRunStoresNothing(t *testing.T) {
	stored := credentialStore(t)
	dir := envProject(t)

	opts := headless(dir, Answers{Name: "my-app"})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Empty(t, *stored, "a dry run must not leave a credential behind")
	assert.Contains(t, string(result.Content), "PLACEHOLDER")
}

// --skip-env means the file carries nothing from .env, so there is nothing to
// store either.
func TestRun_SkipEnvStoresNothing(t *testing.T) {
	stored := credentialStore(t)
	dir := envProject(t)

	_, err := Run(headless(dir, Answers{Name: "my-app", SkipEnv: true}))
	require.NoError(t, err)

	assert.Empty(t, *stored)
}

// A secret classified from a name with an empty assignment has nothing to
// store. Sending an empty value would create a credential that resolves to
// nothing, which fails at the container rather than here.
func TestImportSecrets_NoValueIsReportedNotSent(t *testing.T) {
	stored := credentialStore(t)

	vars, report := importSecrets(
		[]manifest.EnvVar{{Name: "API_TOKEN", Secret: true}},
		Detected{EnvVars: []EnvVar{{Name: "API_TOKEN", Kind: EnvSecret, Value: ""}}},
		"my-app",
	)

	assert.Empty(t, *stored)
	assert.Empty(t, vars[0].CredentialID)
	require.Len(t, report.Failed, 1)
	assert.Contains(t, report.Failed[0].Reason, "no value")
}

// A variable that already names a credential is left alone: it was stored by
// an earlier run, or written by hand, and storing it again would leave the
// first one orphaned.
func TestImportSecrets_AlreadyStoredIsLeftAlone(t *testing.T) {
	stored := credentialStore(t)

	vars, report := importSecrets(
		[]manifest.EnvVar{{Name: "API_TOKEN", Secret: true, CredentialID: "66f0aaaa0000000000000001"}},
		Detected{EnvVars: []EnvVar{{Name: "API_TOKEN", Kind: EnvSecret, Value: "shhh"}}},
		"my-app",
	)

	assert.Empty(t, *stored)
	assert.Equal(t, "66f0aaaa0000000000000001", vars[0].CredentialID)
	assert.False(t, report.Any())
}

// An unnamed workload still produces a usable credential name rather than an
// empty prefix and a stray slash.
func TestCredentialName_WithoutAWorkloadName(t *testing.T) {
	assert.Equal(t, "my-app/OPENAI_API_KEY", CredentialName("my-app", "OPENAI_API_KEY"))
	assert.Equal(t, "OPENAI_API_KEY", CredentialName("", "OPENAI_API_KEY"))
}

// Interactively the secrets are stored when the confirm screen is accepted,
// and the file that gets written is the one that was on screen with the
// placeholders resolved.
func TestFlow_ConfirmStoresTheSecrets(t *testing.T) {
	stored := credentialStore(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	// Name, kind, source, readiness, env: the confirm screen is next.
	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)
	assert.Contains(t, string(model.content), "PLACEHOLDER", "nothing has been stored while it can still be cancelled")

	accepted := press(t, model, "enter")

	require.Len(t, *stored, 1)
	assert.Equal(t, "test-app/OPENAI_API_KEY", (*stored)[0].Name)
	assert.True(t, accepted.done)
	assert.Contains(t, string(accepted.content), "dr-credential:66f000000000000000000001/apiToken")
	assert.NotContains(t, string(accepted.content), "PLACEHOLDER")
}

// Cancelling after walking the whole wizard must leave nothing behind: a
// credential outlives the run that created it.
func TestFlow_CancelStoresNothing(t *testing.T) {
	stored := credentialStore(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	_, _, err := press(t, model, "esc").result()
	require.Error(t, err)
	assert.Empty(t, *stored)
}

// --dry-run promises to change nothing, and a credential is a change that
// outlives the run. The interactive path has to mean by it what the headless
// one does.
func TestFlow_DryRunStoresNothing(t *testing.T) {
	stored := credentialStore(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	start := newFlow(Detect(dir), nil, Answers{})
	start.dryRun = true

	model := press(t, pastName(t, start), "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	accepted := press(t, model, "enter")

	assert.Empty(t, *stored, "a run that writes no file must create no credential")
	assert.True(t, accepted.done)
	assert.Contains(t, string(accepted.content), "PLACEHOLDER",
		"with nothing stored there is no id to write, and the entry stays visibly unfinished")
}

// The confirm screen's [e] is the last chance to change the file, and the
// secrets are stored after it. Re-rendering from the draft at that point threw
// the edit away, silently, in the one step between agreeing and the write.
func TestFlow_ConfirmKeepsAHandEditWhenSecretsAreStored(t *testing.T) {
	stored := credentialStore(t)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz01\n"), 0o600))

	model := press(t, pastName(t, newFlow(Detect(dir), nil, Answers{})), "enter", "enter", "enter", "enter")
	require.Equal(t, screenConfirm, model.at)

	// What the editor left: the rendered file with one line the wizard would
	// never produce.
	edit := strings.Replace(string(model.content), "importance: low", "importance: high", 1)
	require.NotEqual(t, string(model.content), edit, "the fixture has to actually change something")

	edited, _ := model.edited(editedMsg{content: []byte(edit)})
	model, ok := edited.(flow)
	require.True(t, ok)

	accepted := press(t, model, "enter")
	require.Len(t, *stored, 1)

	assert.Contains(t, string(accepted.content), "importance: high", "the hand edit survives to the file")
	assert.Contains(t, string(accepted.content), "dr-credential:66f000000000000000000001/apiToken",
		"and the credential the run stored is still the one referenced")
	assert.NotContains(t, string(accepted.content), "PLACEHOLDER")
}
