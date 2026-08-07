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

package access

import (
	"github.com/datarobot/cli/cmd/enclave/access/grant"
	"github.com/datarobot/cli/cmd/enclave/access/revoke"
	"github.com/datarobot/cli/cmd/enclave/access/show"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "access",
		Short: "Inspect and manage who can access an enclave.",
		Long: `Inspect enclave access with "show", or grant/revoke a role (owner, user,
consumer) for a user, group, or organization.

These commands are thin wrappers over the enclave sharedRoles API. They take
effect only when the server has ENCLAVE_RBAC_ENABLED=true; otherwise the call
succeeds but grants nothing. The caller must hold CAN_SHARE on the enclave;
system administrators may manage any enclave.

To control who may create enclaves at all, see "dr enclave permission".`,
	}

	cmd.AddCommand(
		show.Cmd(),
		grant.Cmd(),
		revoke.Cmd(),
	)

	return cmd
}
