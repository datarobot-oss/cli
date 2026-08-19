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
	"github.com/datarobot/cli/internal/cli"
	"github.com/spf13/cobra"
)

// StampInteractionMode annotates props with whether the current invocation is running
// in non-interactive mode. This happens once in root PersistentPreRunE so every event
// shares the same interaction metadata without per-command duplication.
//
// TODO: fold additional universal automation flags (e.g. future --non-interactive variants)
//
//	into this helper instead of re-implementing the logic in individual commands.
func StampInteractionMode(props *CommonProperties, cmd *cobra.Command) {
	if props == nil {
		return
	}

	props.NonInteractive = cli.IsNonInteractive(cmd)
}
