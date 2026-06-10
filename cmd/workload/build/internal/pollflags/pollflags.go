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
// --poll-timeout flags shared between `dr workload build trigger` and
// `dr workload build get`. Single source of truth so the two commands
// cannot drift out of sync on defaults or names.

package pollflags

import (
	"time"

	"github.com/spf13/cobra"
)

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
// (hidden) to cmd, binding them into s. Returns s for chaining.
func Register(cmd *cobra.Command, s *Set) *Set {
	cmd.Flags().BoolVar(&s.Wait, "wait", false, "Poll until the build reaches a terminal status.")
	cmd.Flags().DurationVar(&s.Interval, "poll-interval", DefaultPollInterval, "Interval between status polls.")
	cmd.Flags().DurationVar(&s.Timeout, "poll-timeout", DefaultPollTimeout, "Maximum time to wait before giving up.")
	_ = cmd.Flags().MarkHidden("poll-interval")
	_ = cmd.Flags().MarkHidden("poll-timeout")

	return s
}
