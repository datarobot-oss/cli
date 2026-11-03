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

package permission

import (
	"github.com/datarobot/cli/cmd/enclave/permission/grant"
	"github.com/datarobot/cli/cmd/enclave/permission/list"
	"github.com/datarobot/cli/cmd/enclave/permission/revoke"
	"github.com/datarobot/cli/cmd/enclave/permission/show"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permission",
		Short: "Inspect and manage who can create enclaves, pin workloads or use the Hub.",
		Long: `Inspect ("show", "list") or manage ("grant", "revoke") collection-level enclave
permissions — capabilities that are not tied to any single enclave.

There are three, and they are independent (none implies another):
  create   the right to register a new enclave. A system administrator grants
           it to org admins and other users, so that creating enclaves is
           delegated as a permission rather than inferred from a role.
  pin      the right to pin a workload to one chosen enclave
           ("dr workload create --enclave"), overriding the scheduler. Granted
           to users and groups (not organizations), by a system administrator
           (any user or group) or an org admin (users and groups in their own
           organization). A group grant lets its members pin. A system
           administrator also needs a pin grant to pin.
  hub      the right to run workloads on the Hub. Checked only when the
           organization requires a use case: a workload in a use case without
           Enclaves runs on the Hub and needs it. Granted like pin: to users and
           groups, by a system administrator or an org admin in their own
           organization.

Use "dr enclave access" instead to manage access to an existing enclave.

These commands take effect only when the server has ENCLAVE_RBAC_ENABLED=true;
otherwise create and pin succeed but change nothing, and hub is refused. Granting
and revoking create require a system administrator; pin and hub can also be
managed by an org admin, for users and groups in their own organization.`,
	}

	cmd.AddCommand(
		list.Cmd(),
		show.Cmd(),
		grant.Cmd(),
		revoke.Cmd(),
	)

	return cmd
}
