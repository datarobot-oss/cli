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

// A check returns a remedy verbatim, so the text reporter prints a shared
// remedy once.
const (
	remedyCheckCredentials  = "Check that the install is reachable from here, then check your credentials: DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN, or run 'dr auth login' for the active profile."
	remedyRetry             = "Run the check again."
	remedyCheckNetwork      = "Run the check again, then read the message and check the network path to the install."
	remedyCustomApplication = "Deploy a Custom Application instead, or ask an install administrator whether the install has the Workload API."
	remedyInternalError     = "Run the check again, and report the message in the details as a bug in dr if it repeats."
	remedyEnableProxy       = "Ask an install administrator to check that the Workload API proxy is switched on and that the gateway reaches it."
	remedyGrantSeat         = "Ask an install administrator for the Agentic + Predictive + Governance Builder seat."
	remedyEnableFlag        = "Ask an install administrator to enable ENABLE_WORKLOAD_API_CONTAINERS for your account."
	remedyReadMessage       = "Read the message in the details, then ask an install administrator to look into it."
)
