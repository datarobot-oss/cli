// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package dotenv

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- needsPulumiSetup matrix ---
// Full coverage of all login/passphrase/prompt combinations.

func TestNeedsPulumiSetup_NoPulumiPrompt(t *testing.T) {
	prompts := []envbuilder.UserPrompt{
		{Env: "DATAROBOT_ENDPOINT", Active: true},
		{Env: "DATAROBOT_API_TOKEN", Active: true},
	}

	assert.False(t, needsPulumiSetup(prompts, false, false))
}

func TestNeedsPulumiSetup_InactivePrompt(t *testing.T) {
	prompts := []envbuilder.UserPrompt{
		{Env: "PULUMI_CONFIG_PASSPHRASE", Active: false},
	}

	assert.False(t, needsPulumiSetup(prompts, false, false))
}

func TestNeedsPulumiSetup_HiddenPrompt(t *testing.T) {
	prompts := []envbuilder.UserPrompt{
		{Env: "PULUMI_CONFIG_PASSPHRASE", Active: true, Hidden: true},
	}

	assert.False(t, needsPulumiSetup(prompts, false, false))
}

func TestNeedsPulumiSetup_NotLoggedIn_NoPassphrase(t *testing.T) {
	prompts := []envbuilder.UserPrompt{{Env: "PULUMI_CONFIG_PASSPHRASE", Active: true}}

	assert.True(t, needsPulumiSetup(prompts, false, false), "not logged in + no passphrase → needs setup")
}

func TestNeedsPulumiSetup_NotLoggedIn_PassphraseSet(t *testing.T) {
	prompts := []envbuilder.UserPrompt{{Env: "PULUMI_CONFIG_PASSPHRASE", Active: true}}

	assert.True(t, needsPulumiSetup(prompts, false, true), "not logged in + passphrase set → still needs setup (login required)")
}

func TestNeedsPulumiSetup_LoggedIn_NoPassphrase(t *testing.T) {
	prompts := []envbuilder.UserPrompt{{Env: "PULUMI_CONFIG_PASSPHRASE", Active: true}}

	assert.True(t, needsPulumiSetup(prompts, true, false), "logged in + no passphrase → needs setup")
}

func TestNeedsPulumiSetup_LoggedIn_PassphraseSet(t *testing.T) {
	prompts := []envbuilder.UserPrompt{{Env: "PULUMI_CONFIG_PASSPHRASE", Active: true}}

	assert.False(t, needsPulumiSetup(prompts, true, true), "logged in + passphrase set → no setup needed")
}

// --- Model initial screen ---

func TestPulumiLoginModel_NotLoggedIn_StartsAtBackendSelection(t *testing.T) {
	model := newPulumiLoginModel(false, false)

	assert.Equal(t, pulumiLoginScreenBackendSelection, model.currentScreen)
	assert.Equal(t, 0, model.selectedOption)
	assert.Len(t, model.options, 3)
}

func TestPulumiLoginModel_AlreadyLoggedIn_StartsAtPassphraseScreen(t *testing.T) {
	model := newPulumiLoginModel(true, true)

	assert.Equal(t, pulumiLoginScreenPassphrasePrompt, model.currentScreen)
}

// --- The key regression test ---
// When not logged in AND passphrase is needed, login must come FIRST.
// The passphrase screen must NOT appear before the login command runs.

func TestPulumiLoginModel_NotLoggedIn_NeedsPassphrase_LoginBeforePassphrase(t *testing.T) {
	model := newPulumiLoginModel(false, true)

	assert.Equal(t, pulumiLoginScreenBackendSelection, model.currentScreen, "must start at backend selection, not passphrase")

	// Press enter on local — must go to logging-in screen, not passphrase
	updated, loginCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	plm, ok := updated.(pulumiLoginModel)
	require.True(t, ok)
	assert.Equal(t, pulumiLoginScreenLoggingIn, plm.currentScreen, "must show logging-in before passphrase prompt")
	assert.NotNil(t, loginCmd, "must have a login command")

	// Simulate login success — now the passphrase screen should appear
	updated, _ = plm.Update(pulumiLoginSuccessMsg{output: "ok"})

	plm, ok = updated.(pulumiLoginModel)
	require.True(t, ok)
	assert.Equal(t, pulumiLoginScreenPassphrasePrompt, plm.currentScreen, "passphrase screen must appear after login, not before")
}

func TestPulumiLoginModel_NotLoggedIn_NoPassphrase_LoginThenComplete(t *testing.T) {
	model := newPulumiLoginModel(false, false)

	// Press enter on local
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	plm, ok := updated.(pulumiLoginModel)
	require.True(t, ok)
	assert.Equal(t, pulumiLoginScreenLoggingIn, plm.currentScreen)

	// Login success → complete immediately, no passphrase screen
	_, cmd := plm.Update(pulumiLoginSuccessMsg{output: "ok"})
	require.NotNil(t, cmd)

	msg := cmd()
	_, isComplete := msg.(pulumiLoginCompleteMsg)
	assert.True(t, isComplete, "must complete without showing passphrase screen")
}

// --- Backend selection key handling ---

func TestPulumiLoginModel_BackendSelection_NavigateUpDown(t *testing.T) {
	model := newPulumiLoginModel(false, false)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	plm := updated.(pulumiLoginModel)
	assert.Equal(t, 1, plm.selectedOption)

	updated, _ = plm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	plm = updated.(pulumiLoginModel)
	assert.Equal(t, 0, plm.selectedOption)

	// Can't go below 0
	updated, _ = plm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	plm = updated.(pulumiLoginModel)
	assert.Equal(t, 0, plm.selectedOption)
}

func TestPulumiLoginModel_BackendSelection_DIY_GoesToDIYURLScreen(t *testing.T) {
	model := newPulumiLoginModel(false, false)

	// Navigate to DIY (option 2)
	model.selectedOption = 2

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	plm := updated.(pulumiLoginModel)
	assert.Equal(t, pulumiLoginScreenDIYURL, plm.currentScreen)
}

func TestPulumiLoginModel_DIYURLScreen_Esc_ReturnsToBackendSelection(t *testing.T) {
	model := newPulumiLoginModel(false, false)
	model.currentScreen = pulumiLoginScreenDIYURL

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	plm := updated.(pulumiLoginModel)
	assert.Equal(t, pulumiLoginScreenBackendSelection, plm.currentScreen)
}

func TestPulumiLoginModel_DIYURLScreen_EmptyURL_DoesNotProceed(t *testing.T) {
	model := newPulumiLoginModel(false, false)
	model.currentScreen = pulumiLoginScreenDIYURL
	model.diyInput = textinput.New()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	plm := updated.(pulumiLoginModel)
	assert.Equal(t, pulumiLoginScreenDIYURL, plm.currentScreen, "empty URL must not proceed")
	assert.Nil(t, cmd)
}

// --- Passphrase screen exit paths ---
// All three ways out (y, n, esc) must be tested — any one of them breaking
// will leave the user stuck on the passphrase screen.

func TestPulumiLoginModel_PassphraseScreen_N_Completes(t *testing.T) {
	model := newPulumiLoginModel(true, true)

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.NotNil(t, cmd)

	msg := cmd()
	_, isComplete := msg.(pulumiLoginCompleteMsg)
	assert.True(t, isComplete, "pressing n must complete the Pulumi setup")
}

func TestPulumiLoginModel_PassphraseScreen_UpperN_Completes(t *testing.T) {
	model := newPulumiLoginModel(true, true)

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	require.NotNil(t, cmd)

	msg := cmd()
	_, isComplete := msg.(pulumiLoginCompleteMsg)
	assert.True(t, isComplete)
}

func TestPulumiLoginModel_PassphraseScreen_Esc_Completes(t *testing.T) {
	model := newPulumiLoginModel(true, true)

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, isComplete := msg.(pulumiLoginCompleteMsg)
	assert.True(t, isComplete, "pressing esc must complete the Pulumi setup")
}

// --- Passphrase generation ---

func TestGenerateRandomPassphrase(t *testing.T) {
	passphrase, err := generateRandomSecret(32)
	require.NoError(t, err)
	assert.Len(t, passphrase, 32)

	passphrase2, err := generateRandomSecret(32)
	require.NoError(t, err)
	assert.NotEqual(t, passphrase, passphrase2, "generated passphrases must be unique")
}
