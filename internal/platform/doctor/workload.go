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

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/platform"
)

const (
	idWorkloadDeploy = "platform.workload.deploy"

	// builderSeat is the one seat the monolith proxy and the Workload API both
	// require. A seat that is absent from the report is not enforced.
	builderSeat = "AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER"

	// containersFlag is the feature flag both layers require.
	containersFlag = "ENABLE_WORKLOAD_API_CONTAINERS"
)

// workloadDeploy says whether the Workload API accepts the caller.
type workloadDeploy struct {
	snapshot *snapshot
}

// ID returns the stable check ID.
func (c *workloadDeploy) ID() string { return idWorkloadDeploy }

// Name returns the display name of the check.
func (c *workloadDeploy) Name() string { return "Workload deploy access" }

// Run reads the shared report and judges the workloadApi section.
func (c *workloadDeploy) Run(ctx context.Context) core.Result {
	report, err := c.snapshot.get(ctx)
	if err != nil {
		return core.Result{Status: core.StatusSKIP, Summary: "The install report is missing, see " + idInstallRead + "."}
	}

	return evaluateWorkloadDeploy(report)
}

// evaluateWorkloadDeploy judges the workloadApi section, which is the live
// answer of the gate, and uses the seats and entitlements sections only to
// name the cause of a 403.
func evaluateWorkloadDeploy(report *platform.Report) core.Result {
	section, ok := report.Sections[platform.SectionWorkloadAPI]

	switch {
	case !ok:
		return core.Result{Status: core.StatusSKIP, Summary: "The report has no workloadApi section."}
	case section.Status == platform.StatusOK:
		return core.Result{Status: core.StatusOK, Summary: "The Workload API answers this caller."}
	case section.HTTPStatus == 0:
		return core.Result{
			Status:  core.StatusWARN,
			Summary: "The Workload API route could not be read: " + section.Message,
			Remedy:  remedyRetry,
		}
	}

	return explainRefusal(report, section)
}

// explainRefusal maps a refused workloadApi section to its cause and remedy.
func explainRefusal(report *platform.Report, section platform.Section) core.Result {
	result := core.Result{
		Status: core.StatusFAIL,
		Details: map[string]string{
			"httpStatus": strconv.Itoa(section.HTTPStatus),
			"message":    section.Message,
		},
	}

	switch {
	case section.HTTPStatus == http.StatusNotFound:
		result.Status = core.StatusWARN
		result.Summary = "This release has no Workload API."
		result.Remedy = remedyCustomApplication
	case section.HTTPStatus == http.StatusUnauthorized:
		result.Summary = "The Workload API rejected the credentials."
		result.Remedy = remedyCheckCredentials
	case section.HTTPStatus == http.StatusBadGateway:
		result.Summary = "The Workload API proxy is switched off, or a gateway in front of it failed."
		result.Remedy = remedyEnableProxy
	case section.HTTPStatus == http.StatusForbidden && seatMissing(report):
		result.Summary = "The caller has no Agentic + Predictive + Governance Builder seat."
		result.Remedy = remedyGrantSeat
	case section.HTTPStatus == http.StatusForbidden && flagOff(report):
		result.Summary = containersFlag + " is off for the caller."
		result.Remedy = remedyEnableFlag
	default:
		result.Summary = fmt.Sprintf("The Workload API answered HTTP %d.", section.HTTPStatus)
		result.Remedy = remedyReadMessage
	}

	return result
}

// seatMissing reports whether the install enforces the builder seat and the
// caller lacks it.
func seatMissing(report *platform.Report) bool {
	seats, ok := report.Sections[platform.SectionSeats].Data.(platform.Seats)
	if !ok {
		return false
	}

	held, enforced := seats.SeatLicenses[builderSeat]

	return enforced && !held
}

// flagOff reports whether the entitlements section says the containers flag is
// false for the caller.
func flagOff(report *platform.Report) bool {
	flags, ok := report.Sections[platform.SectionEntitlements].Data.(map[string]bool)
	if !ok {
		return false
	}

	value, known := flags[containersFlag]

	return known && !value
}
