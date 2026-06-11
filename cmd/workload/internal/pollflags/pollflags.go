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

// Package pollflags centralizes the --wait, --poll-interval, and
// --poll-timeout flags shared between the polling workload commands
// (`dr workload build trigger`, `dr workload build get`, and
// `dr workload status`). Single source of truth for the flag names and
// registration so the commands cannot drift out of sync; poll defaults
// and the --wait help text vary per command via RegisterWithDefaults.

package pollflags

import (
	"time"

	"github.com/spf13/cobra"
)

// Build-oriented defaults: container image builds are slow, so callers that
// do not pass their own defaults poll every couple of seconds for up to half
// an hour. Commands whose work settles faster pass tighter values via
// RegisterWithDefaults.
const (
	DefaultPollInterval = 2 * time.Second
	DefaultPollTimeout  = 30 * time.Minute
)

// Set holds the resolved values from a poll-flag triple. Callers register
// flags with Register and then read with the accessors so the flag names
// stay local to this package.
type Set struct {
	Wait     bool
	Interval time.Duration
	Timeout  time.Duration
}

// Register adds --wait, --poll-interval (hidden), and --poll-timeout
// (hidden) to cmd with the build-oriented defaults and --wait help text,
// binding them into s. Returns s for chaining.
func Register(cmd *cobra.Command, s *Set) *Set {
	return RegisterWithDefaults(cmd, s, DefaultPollInterval, DefaultPollTimeout,
		"Poll until the build reaches a terminal status.")
}

// RegisterWithDefaults is Register with caller-chosen interval and timeout
// defaults plus the --wait help text, for commands whose work settles on a
// different timescale or vocabulary than a container build (e.g.
// `dr workload status --wait` settles in minutes, and on steady states
// like running that are not terminal).
func RegisterWithDefaults(cmd *cobra.Command, s *Set, interval, timeout time.Duration, waitUsage string) *Set {
	cmd.Flags().BoolVar(&s.Wait, "wait", false, waitUsage)
	cmd.Flags().DurationVar(&s.Interval, "poll-interval", interval, "Interval between status polls.")
	cmd.Flags().DurationVar(&s.Timeout, "poll-timeout", timeout, "Maximum time to wait before giving up.")
	_ = cmd.Flags().MarkHidden("poll-interval")
	_ = cmd.Flags().MarkHidden("poll-timeout")

	return s
}
