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
		Short: "Inspect and manage who can create enclaves.",
		Long: `Inspect ("show", "list") or manage ("grant", "revoke") collection-level enclave
permissions — capabilities that are not tied to any single enclave.

Today the only such permission is "create": the right to register a new
enclave. A system administrator grants it to org admins and other users, so
that creating enclaves is delegated as a permission rather than inferred from a
role.

Use "dr enclave access" instead to manage access to an existing enclave.

These commands take effect only when the server has ENCLAVE_RBAC_ENABLED=true;
otherwise the call succeeds but changes nothing. Granting and revoking require
a system administrator.`,
	}

	cmd.AddCommand(
		list.Cmd(),
		show.Cmd(),
		grant.Cmd(),
		revoke.Cmd(),
	)

	return cmd
}
