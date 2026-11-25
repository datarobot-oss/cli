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

package doctor

// The canonical remedy for each condition. A check returns the constant
// verbatim, so the text reporter prints a shared remedy once.
const (
	remedyCheckCredentials  = "Check that the install is reachable from here, then check your credentials: DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN, or run 'dr auth login' for the active profile."
	remedyRetry             = "Run the check again, then check the network path to the install."
	remedyCustomApplication = "Deploy a Custom Application instead, or ask an install administrator for a release that has the Workload API."
	remedyEnableProxy       = "Ask an install administrator to switch the Workload API proxy on."
	remedyGrantSeat         = "Ask an install administrator for the Agentic + Predictive + Governance Builder seat."
	remedyEnableFlag        = "Ask an install administrator to enable ENABLE_WORKLOAD_API_CONTAINERS for your account."
	remedyReadMessage       = "Read the message in the details, then ask an install administrator to fix the check it names."
)
