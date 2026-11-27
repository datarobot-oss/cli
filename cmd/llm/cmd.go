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

// Package llm implements `dr llm`: deploying a recipe as an llm workload,
// rolling it onto another recipe version, and chatting with it.
//
// `llm` used to be an alias of `dr llm-gateway`. Hidden `list` and `select`
// forward to that group so `dr llm list|select` keep working, and they are
// registered whatever the feature gate says; only the new commands are gated.
package llm

import (
	gatewaylist "github.com/datarobot/cli/cmd/llm-gateway/list"
	gatewayselect "github.com/datarobot/cli/cmd/llm-gateway/select"
	"github.com/datarobot/cli/cmd/llm/chat"
	"github.com/datarobot/cli/cmd/llm/deploy"
	"github.com/datarobot/cli/cmd/llm/rollout"
	wlendpoint "github.com/datarobot/cli/cmd/workload/endpoint"
	wlstatus "github.com/datarobot/cli/cmd/workload/status"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/features"
	llmapi "github.com/datarobot/cli/internal/llm"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "llm",
		GroupID: "core",
		Short:   "🧠 Deploy and use LLMs served from recipes",
		Long: `Deploy LLMs on the Dynamo operator from recipes, roll them onto new
recipe versions, and chat with them.

  dr llm deploy --recipe <id> --name <name> --bundle <bundle-id> --wait
  dr llm chat <workload-id> "Hello"
  dr llm rollout <workload-id> --recipe-version 3 --wait

status, watch and endpoint run the matching 'dr workload' commands; logs,
settings, stop, start and delete are 'dr workload' commands too.

Recipes are managed with 'dr recipe'. LLM serving needs the installation's
switch and the ENABLE_WORKLOAD_API_TOKEN_FACTORY feature flag on your user,
organization or group. The commands are shown when
DATAROBOT_CLI_FEATURE_WORKLOAD_API_LLM=true is set.

The LLM Gateway catalog is 'dr llm-gateway'; 'dr llm list' and
'dr llm select' still forward there.`,
		// Nothing to show until the gate is on: only the hidden forwards
		// are registered without it.
		Hidden: !features.Enabled(llmapi.Gate),
	}

	adder := &cli.CommandAdder{Command: cmd}
	adder.AddCommand(
		gated(deploy.Cmd()),
		gated(rollout.Cmd()),
		gated(chat.Cmd()),
		gated(alias(wlstatus.Cmd(), "Show an LLM workload's status (same as 'dr workload status').")),
		gated(watchCmd()),
		gated(alias(wlendpoint.Cmd(), "Print an LLM workload's endpoint URL (same as 'dr workload endpoint').")),
		forward(gatewaylist.Cmd()),
		forward(gatewayselect.Cmd()),
	)

	return cmd
}

func gated(c *cobra.Command) *cobra.Command {
	features.SetGate(c, llmapi.Gate)

	return c
}

// alias is a fresh instance of a `dr workload` command under `dr llm`: the
// same arguments, flags, output and implementation, by construction.
func alias(c *cobra.Command, short string) *cobra.Command {
	c.Short = short

	return c
}

// watchCmd is `dr workload status --watch` under the name `watch`.
func watchCmd() *cobra.Command {
	c := wlstatus.Cmd()
	c.Use = "watch [<workload-id>]"
	c.Short = "Follow an LLM workload's status until it settles (same as 'dr workload status --watch')."

	if f := c.Flags().Lookup("watch"); f != nil {
		_ = f.Value.Set("true")
		f.DefValue = "true"
		f.Hidden = true
	}

	return c
}

// forward keeps a former `dr llm <verb>` (the llm-gateway alias) working
// without listing it under `dr llm`.
func forward(c *cobra.Command) *cobra.Command {
	c.Hidden = true

	return c
}
