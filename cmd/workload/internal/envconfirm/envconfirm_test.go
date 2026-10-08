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

package envconfirm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The wizard reads nil as consent, so --yes must hand back nil rather than a
// function that quietly answers no: that would turn the flag into one that
// skips the work rather than the question.
func TestAsk_YesHandsBackNoQuestion(t *testing.T) {
	assert.Nil(t, Ask(&strings.Builder{}, strings.NewReader(""), Policy{Yes: true}))
}

func TestAsk_YesApplies(t *testing.T) {
	out := &strings.Builder{}

	agreed, err := Ask(out, strings.NewReader("y\n"), Policy{Interactive: true})()
	require.NoError(t, err)

	assert.True(t, agreed)
	assert.Contains(t, out.String(), "Apply this?")
}

// Anything but yes is no, because the two things this agrees to cannot be
// taken back: a file you commit, and a value on the tenant.
func TestAsk_AnythingElseIsNo(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "no\n", "later\n", "Y es\n"} {
		agreed, err := Ask(&strings.Builder{}, strings.NewReader(answer), Policy{Interactive: true})()
		require.NoError(t, err)

		assert.False(t, agreed, "answered %q", answer)
	}
}

// Case and spacing are the user being human, not the user saying no.
func TestAsk_AcceptsYesHoweverItIsTyped(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "  YES  \n"} {
		agreed, err := Ask(&strings.Builder{}, strings.NewReader(answer), Policy{Interactive: true})()
		require.NoError(t, err)

		assert.True(t, agreed, "answered %q", answer)
	}
}

// A closed stdin is not consent. It is what a pipeline that reached this
// prompt by mistake looks like, and the safe reading is that nothing happens.
func TestAsk_UnreadableAnswerIsNo(t *testing.T) {
	agreed, err := Ask(&strings.Builder{}, strings.NewReader(""), Policy{Interactive: true})()
	require.NoError(t, err)

	assert.False(t, agreed)
}

// --yes is consent and is the only thing that is, so it never stops.
func TestAsk_YesNeedsNoTerminal(t *testing.T) {
	assert.Nil(t, Ask(&strings.Builder{}, strings.NewReader(""), Policy{Yes: true}))
}

// A dry run writes nothing and sends nothing, so there is nothing to agree to.
// It still shows the table, which the wizard prints either way.
func TestAsk_DryRunNeedsNoConsent(t *testing.T) {
	assert.Nil(t, Ask(&strings.Builder{}, strings.NewReader(""), Policy{DryRun: true}))
}

// A pipeline that got here without saying yes has agreed to nothing. Applying
// silently would make a scheduled deploy the one caller that overwrites tenant
// secrets with no record of having been allowed to.
func TestAsk_NoTerminalWithoutYesRefuses(t *testing.T) {
	ask := Ask(&strings.Builder{}, strings.NewReader(""), Policy{Interactive: false})
	require.NotNil(t, ask, "a refusal has to be voiced, not left as silent consent")

	agreed, err := ask()
	require.Error(t, err)

	assert.False(t, agreed)
	assert.Contains(t, err.Error(), "--yes", "the refusal names the way out of it")
	assert.Contains(t, err.Error(), "credential store", "and what it was protecting")
}
