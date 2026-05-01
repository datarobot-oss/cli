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

// Package telemetry provides anonymous usage analytics for the DataRobot CLI.
//
// Telemetry is collected via the Amplitude analytics-go SDK. When telemetry is
// disabled (via --disable-telemetry flag, DATAROBOT_CLI_DISABLE_TELEMETRY env var,
// or disable-telemetry config key) or the Amplitude API key is not set (dev builds),
// all tracking operations are safe no-ops that log events to the debug logger instead.
//
// The package is designed to never block CLI execution or cause errors visible to users.
package telemetry

import (
	"time"

	"github.com/amplitude/analytics-go/amplitude"
	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

// AmplitudeAPIKey is set at build time via ldflags. Dev builds have an empty value,
// which automatically disables telemetry.
var AmplitudeAPIKey string

// InstallMethod is set at build time via ldflags. Dev builds default to "source".
var InstallMethod string = "source"

// Client wraps the Amplitude SDK client. When telemetry is disabled or the
// API key is empty, all methods are safe no-ops that log to the debug logger.
type Client struct {
	amp   amplitude.Client
	props *CommonProperties
}

// amplitudeLogger adapts the internal log package to Amplitude's Logger interface.
type amplitudeLogger struct{}

func (l *amplitudeLogger) Debugf(msg string, args ...any) { log.Debugf(msg, args...) }
func (l *amplitudeLogger) Infof(msg string, args ...any)  { log.Infof(msg, args...) }
func (l *amplitudeLogger) Warnf(msg string, args ...any)  { log.Warnf(msg, args...) }
func (l *amplitudeLogger) Errorf(msg string, args ...any) { log.Errorf(msg, args...) }

// NewClient creates a telemetry client. If IsEnabled() returns true, it initializes
// a real Amplitude client. Otherwise, it returns a no-op client that logs
// events via the internal/log debug logger for development visibility.
func NewClient(props *CommonProperties) *Client {
	if !IsEnabled() {
		return &Client{amp: nil, props: props}
	}

	config := amplitude.NewConfig(AmplitudeAPIKey)
	config.Logger = &amplitudeLogger{}

	client := amplitude.NewClient(config)

	return &Client{
		amp:   client,
		props: props,
	}
}

// Track queues an event for delivery to Amplitude. Common properties from the
// client's CommonProperties are merged into the event's EventProperties before
// sending. UserID is set as a top-level event field (required by Amplitude).
// This call is non-blocking. When the client is a no-op (dev builds),
// the event is logged via log.Debug instead.
func (c *Client) Track(event types.Event) {
	if c.amp == nil {
		log.Debug("Telemetry event (dry-run)", "type", event.EventType, "properties", event.EventProperties)
		return
	}

	// Merge common properties into event properties
	if c.props != nil {
		commonMap := c.props.AsMap()
		for k, v := range event.EventProperties {
			commonMap[k] = v
		}

		event.EventProperties = commonMap

		// Set UserID and DeviceID as top-level fields (required by Amplitude)
		event.UserID = c.props.UserID
		event.DeviceID = c.props.DeviceID
	}

	c.amp.Track(event)
}

// Flush sends all queued events and blocks until delivery completes or the
// timeout elapses. Should be called once at process exit (typically in
// PersistentPostRunE). Safe to call on a no-op client.
func (c *Client) Flush(timeout time.Duration) {
	if c.amp == nil {
		return
	}

	done := make(chan struct{})

	go func() {
		c.amp.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Successfully flushed
	case <-time.After(timeout):
		// Timeout reached, events may not have been sent
		log.Debug("Telemetry flush timed out", "timeout", timeout)
	}
}

// IsEnabled reports whether telemetry collection is active. Returns true only
// when the Amplitude API key is set (non-dev build) and the user has not
// opted out via the disable-telemetry flag/env var/config key.
func IsEnabled() bool {
	// Check if telemetry is explicitly disabled
	if viperx.GetBool("disable-telemetry") {
		return false
	}

	// Check if API key is set (dev builds won't have one)
	if AmplitudeAPIKey == "" {
		return false
	}

	return true
}
