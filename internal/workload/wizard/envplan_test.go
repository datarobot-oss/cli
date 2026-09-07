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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asking is a headless sync that answers the table with agreed. The table
// itself goes to Stderr, because a dry run has nobody to ask and still has to
// show it.
func asking(dir string, agreed bool, shown *bytes.Buffer) Options {
	opts := syncing(dir, Answers{})
	opts.Stderr = shown
	opts.Confirm = func() (bool, error) { return agreed, nil }

	return opts
}

// Every kind of variable in one plan, because the reader's question is what
// happens to a name rather than which category it fell into, and a name that
// appears under no heading at all is the one they will not think to ask about.
func TestEnvPlan_SortsEveryVariableIntoWhatHappensToIt(t *testing.T) {
	credentialStore(t)

	dir := configured(t, "GREETING=hello\nOLD=gone\n")
	writeEnvFile(t, dir, "GREETING=hello there\n"+
		"REGION=eu-west-1\n"+
		"STRIPE_API_KEY=fixture-not-a-real-key-1a2b3c4d\n"+
		"DATABASE_URL=postgres://localhost:5432/dev\n")

	shown := &bytes.Buffer{}

	_, err := Run(asking(dir, false, shown))
	require.NoError(t, err)

	assert.Contains(t, shown.String(), "GREETING", "a declared value that moved")
	assert.Contains(t, shown.String(), "REGION", "a name .env has and the manifest does not")
	assert.Contains(t, shown.String(), "STRIPE_API_KEY", "a new name the classifier reads as a secret")
	assert.Contains(t, shown.String(), "DATABASE_URL", "a name held back as local-only")

	// The half that loses configuration rather than gaining it, named rather
	// than left to be discovered in the diff.
	assert.Contains(t, shown.String(), "OLD")
	assert.Contains(t, shown.String(), "remove")
}

// A refusal is a decision, not a failure: neither file is touched, nothing
// reaches the tenant, and the run carries on.
func TestConfirmEnvPlan_RefusalChangesNothing(t *testing.T) {
	dir := configured(t, "GREETING=hello\n")
	writeEnvFile(t, dir, "GREETING=hello there\nREGION=eu-west-1\n")

	before := readManifest(t, dir)

	shown := &bytes.Buffer{}

	result, err := Run(asking(dir, false, shown))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, 0, result.EnvKeysListed)
	assert.Equal(t, 0, result.EnvValuesUpdated)
	assert.Equal(t, before, readManifest(t, dir), "the file is byte for byte what it was")
}

// Agreeing carries out both halves, which is the whole of what one flag means.
func TestConfirmEnvPlan_AgreementAppliesBothHalves(t *testing.T) {
	dir := configured(t, "GREETING=hello\n")
	writeEnvFile(t, dir, "GREETING=hello there\nREGION=eu-west-1\n")

	shown := &bytes.Buffer{}

	result, err := Run(asking(dir, true, shown))
	require.NoError(t, err)

	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, 1, result.EnvKeysListed)
	assert.Equal(t, 1, result.EnvValuesUpdated)
}

// Stopping to ask about work that is not going to happen is how people learn
// to answer without reading.
func TestConfirmEnvPlan_NothingToDoIsNotWorthAsking(t *testing.T) {
	dir := configured(t, "GREETING=hello\n")
	writeEnvFile(t, dir, "GREETING=hello\n")

	opts := syncing(dir, Answers{})
	opts.Confirm = func() (bool, error) {
		t.Fatal("every row of this plan is a skip, so there was nothing to agree to")

		return false, nil
	}

	_, err := Run(opts)
	require.NoError(t, err)
}

// No writer, no prompt: --yes, a dry run and a pipeline all hand over nil, and
// the wizard reads that as consent rather than as a refusal it cannot voice.
func TestConfirmEnvPlan_NilConfirmProceeds(t *testing.T) {
	dir := configured(t, "GREETING=hello\n")
	writeEnvFile(t, dir, "GREETING=hello there\n")

	result, err := Run(syncing(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvValuesUpdated)
}

// Names, never values. The table is the one place both sides of every
// comparison are in scope at once, so it is the easiest place to leak one.
func TestEnvPlan_PrintsNoValues(t *testing.T) {
	dir := configured(t, "GREETING=hello\n")
	writeEnvFile(t, dir, "GREETING=a-secret-looking-value\nREGION=eu-west-1\n")

	shown := &bytes.Buffer{}

	_, err := Run(asking(dir, false, shown))
	require.NoError(t, err)

	assert.NotContains(t, shown.String(), "a-secret-looking-value")
	assert.NotContains(t, shown.String(), "eu-west-1")
	assert.NotContains(t, strings.ToLower(shown.String()), "hello")
}
