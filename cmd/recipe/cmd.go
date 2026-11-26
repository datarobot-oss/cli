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

// Package recipe implements `dr recipe`: the serving recipes `llm` workloads
// deploy with, over the Workload API's /api/v2/llmRecipes/.
package recipe

import (
	"github.com/datarobot/cli/cmd/recipe/copycmd"
	"github.com/datarobot/cli/cmd/recipe/create"
	"github.com/datarobot/cli/cmd/recipe/del"
	"github.com/datarobot/cli/cmd/recipe/deployments"
	"github.com/datarobot/cli/cmd/recipe/get"
	"github.com/datarobot/cli/cmd/recipe/list"
	"github.com/datarobot/cli/cmd/recipe/patch"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

// Gate is the CLI feature gate shared by `dr recipe` and the new `dr llm`
// commands while the server feature is flagged.
const Gate = "workload-api-llm"

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "recipe",
		Aliases: []string{"recipes"},
		GroupID: "core",
		Short:   "🍳 Manage LLM serving recipes",
		Long: `Manage the serving recipes LLM workloads deploy with.

A recipe is a known-good, versioned serving configuration for an open-weights
model on the Dynamo operator: a DynamoGraphDeployment (dynamo.dgd/v1beta1) or a
recipes.vllm.ai document (vllm.recipe/v1). Recipes belong to a tenant; the
creator or a tenant admin can change or delete one, and anyone in the tenant
can copy it.

Recipes need LLM serving enabled on the installation and the
ENABLE_WORKLOAD_API_TOKEN_FACTORY feature flag granted to your user,
organization or group. The command group is shown when
DATAROBOT_CLI_FEATURE_WORKLOAD_API_LLM=true is set.`,
	}

	features.SetGate(cmd, Gate)

	cmd.AddCommand(
		list.Cmd(),
		get.Cmd(),
		create.Cmd(),
		copycmd.Cmd(),
		patch.Cmd(),
		del.Cmd(),
		deployments.Cmd(),
	)

	return cmd
}
