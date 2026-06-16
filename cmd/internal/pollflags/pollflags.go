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
// --poll-timeout flags shared between the long-running build commands
// (`dr artifact build create` and `dr artifact build get`). Single source
// of truth for the flag names and registration so the commands cannot drift
// out of sync; poll defaults and the --wait help text vary per command via
// RegisterWithDefaults. PositiveDuration is also reused by `dr workload logs
// --poll-interval` so its non-positive rejection stays consistent.

package pollflags

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// positiveDurationValue parses like a plain duration flag but rejects zero
// and negative values at parse time: time.Sleep returns immediately for
// them, which would turn a poll loop into a hot loop hammering the API.
type positiveDurationValue struct {
	d *time.Duration
}

// PositiveDuration returns a duration flag value with value as the default,
// for poll cadence flags that must stay positive. Commands outside the
// pollflags triple (e.g. `dr workload logs --poll-interval`) reuse it so the
// validation cannot drift between the polling commands.
func PositiveDuration(p *time.Duration, value time.Duration) pflag.Value {
	*p = value

	return &positiveDurationValue{d: p}
}

func (v *positiveDurationValue) Set(s string) error {
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	if parsed <= 0 {
		return errors.New("must be a positive duration")
	}

	*v.d = parsed

	return nil
}

func (v *positiveDurationValue) String() string {
	return v.d.String()
}

func (v *positiveDurationValue) Type() string {
	return "duration"
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
	cmd.Flags().Var(PositiveDuration(&s.Interval, interval), "poll-interval", "Interval between status polls.")
	cmd.Flags().Var(PositiveDuration(&s.Timeout, timeout), "poll-timeout", "Maximum time to wait before giving up.")
	_ = cmd.Flags().MarkHidden("poll-interval")
	_ = cmd.Flags().MarkHidden("poll-timeout")

	return s
}
