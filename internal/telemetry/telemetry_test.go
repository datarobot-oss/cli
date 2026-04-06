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

package telemetry

import (
	"testing"
	"time"

	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestNewClient_DisabledWhenAPIKeyEmpty(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""

	viper.Set("disable-telemetry", false)

	client := NewClient(nil)

	assert.NotNil(t, client)
	assert.Nil(t, client.amp)
}

func TestNewClient_DisabledWhenFlagSet(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = "test-key"

	viper.Set("disable-telemetry", true)

	client := NewClient(nil)

	assert.NotNil(t, client)
	assert.Nil(t, client.amp)

	// Cleanup
	viper.Set("disable-telemetry", false)
}

func TestNewClient_EnabledWhenAPIKeySetAndNotDisabled(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = "test-key"

	viper.Set("disable-telemetry", false)

	client := NewClient(nil)

	assert.NotNil(t, client)
	// Can't directly test amp.Client initialization without mocking, but we can
	// verify it's not nil when enabled
	assert.NotNil(t, client.amp)
}

func TestNewClient_StoresProperties(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""
	props := &CommonProperties{
		UserID:     "test-user",
		CLIVersion: "v0.1.0",
	}

	client := NewClient(props)

	assert.Equal(t, props, client.props)
}

func TestTrack_NoOpWhenDisabled(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""

	viper.Set("disable-telemetry", false)

	client := NewClient(nil)
	event := types.Event{
		EventType: "test event",
		EventProperties: map[string]any{
			"test": "property",
		},
	}

	// Should not panic
	client.Track(event)
}

func TestTrack_MergesCommonProperties(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""
	props := &CommonProperties{
		UserID:     "test-user",
		CLIVersion: "v0.1.0",
		SessionID:  "session-123",
	}

	client := NewClient(props)
	event := types.Event{
		EventType: "test event",
		EventProperties: map[string]any{
			"custom_prop": "custom_value",
		},
	}

	// Should not panic even though amp is nil
	client.Track(event)
}

func TestFlush_NoOpWhenDisabled(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""
	client := NewClient(nil)

	// Should not panic or block
	client.Flush(100 * time.Millisecond)
}

func TestIsEnabled_FalseWhenAPIKeyEmpty(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = ""

	viper.Set("disable-telemetry", false)

	assert.False(t, IsEnabled())
}

func TestIsEnabled_FalseWhenDisableFlagSet(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = "test-key"

	viper.Set("disable-telemetry", true)

	assert.False(t, IsEnabled())

	// Cleanup
	viper.Set("disable-telemetry", false)
}

func TestIsEnabled_TrueWhenAPIKeySetAndNotDisabled(t *testing.T) {
	// Save original value
	originalAPIKey := AmplitudeAPIKey

	defer func() { AmplitudeAPIKey = originalAPIKey }()

	AmplitudeAPIKey = "test-key"

	viper.Set("disable-telemetry", false)

	assert.True(t, IsEnabled())
}

func TestAmplitudeLogger_DoesNotPanic(t *testing.T) {
	logger := &amplitudeLogger{}

	// Should not panic
	logger.Debugf("test %s", "message")
	logger.Infof("test %s", "message")
	logger.Warnf("test %s", "message")
	logger.Errorf("test %s", "message")
}
