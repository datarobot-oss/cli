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

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// RequireFlagWhenChanged returns an error when flagName was set on the
// command line but requiredFlagName was not. Cobra's MarkFlagsRequiredTogether
// is symmetric; this covers the one-way case where one flag only makes sense
// alongside another.
func RequireFlagWhenChanged(cmd *cobra.Command, flagName, requiredFlagName string) error {
	if cmd.Flags().Changed(flagName) && !cmd.Flags().Changed(requiredFlagName) {
		return fmt.Errorf("--%s requires --%s", flagName, requiredFlagName)
	}

	return nil
}
