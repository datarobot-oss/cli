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

package platformdoctor

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/platform"
)

const (
	idWorkloadDeploy = "platform.workload.deploy"

	// builderSeat is the seat that both layers require.
	builderSeat = "AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER"

	causeSeat = "seat"
	causeFlag = "flag"
)

// workloadDeploy says whether the Workload API accepts the caller.
type workloadDeploy struct {
	snapshot *snapshot
}

func (c *workloadDeploy) ID() string { return idWorkloadDeploy }

func (c *workloadDeploy) Name() string { return "Workload deploy access" }

func (c *workloadDeploy) Run(ctx context.Context) core.Result {
	report, err := c.snapshot.get(ctx)
	if err != nil {
		return core.Result{Status: core.StatusSKIP, Summary: "The install report is missing, see " + idInstallRead + "."}
	}

	return evaluateWorkloadDeploy(report)
}

// evaluateWorkloadDeploy judges the workloadApi section, the live answer of the
// gate.
func evaluateWorkloadDeploy(report *platform.Report) core.Result {
	section, ok := report.Sections[platform.SectionWorkloadAPI]

	switch {
	case !ok:
		return core.Result{Status: core.StatusSKIP, Summary: "The report has no workloadApi section."}
	case section.Status == platform.StatusOK:
		return core.Result{Status: core.StatusOK, Summary: "The Workload API answers this caller."}
	case section.HTTPStatus == 0 && strings.HasPrefix(section.Message, platform.InternalErrorPrefix):
		return core.Result{
			Status:  core.StatusWARN,
			Summary: "dr failed while reading the Workload API route: " + section.Message,
			Remedy:  remedyInternalError,
		}
	case section.HTTPStatus == 0:
		return core.Result{
			Status:  core.StatusWARN,
			Summary: "The Workload API route could not be read: " + section.Message,
			Remedy:  remedyCheckNetwork,
		}
	}

	return explainRefusal(report, section)
}

func explainRefusal(report *platform.Report, section platform.Section) core.Result {
	result := core.Result{
		Status: core.StatusFAIL,
		Details: map[string]string{
			"httpStatus": strconv.Itoa(section.HTTPStatus),
			"message":    section.Message,
		},
	}

	cause := forbiddenCause(report, section)

	switch {
	case section.HTTPStatus == http.StatusNotFound:
		result.Status = core.StatusWARN
		result.Summary = "This install does not serve the Workload API."
		result.Remedy = remedyCustomApplication
	case section.HTTPStatus == http.StatusTooManyRequests:
		result.Status = core.StatusWARN
		result.Summary = "The install is throttling the Workload API probe."
		result.Remedy = remedyRetry
	case section.HTTPStatus == http.StatusUnauthorized:
		result.Summary = "The Workload API rejected the credentials."
		result.Remedy = remedyCheckCredentials
	case section.HTTPStatus == http.StatusBadGateway:
		result.Summary = "The Workload API proxy is switched off, or a gateway in front of it failed."
		result.Remedy = remedyEnableProxy
	case cause == causeSeat:
		result.Summary = "The caller has no Agentic + Predictive + Governance Builder seat."
		result.Remedy = remedyGrantSeat
	case cause == causeFlag:
		result.Summary = platform.EntitlementWorkloadAPIContainers + " is off for the caller."
		result.Remedy = remedyEnableFlag
	default:
		result.Summary = fmt.Sprintf("The Workload API answered HTTP %d.", section.HTTPStatus)
		result.Remedy = remedyReadMessage
	}

	return result
}

// forbiddenCause names what a 403 is about, or "" for any other answer. Both
// layers name the seat license or the feature flag in the message, so it
// decides before the seats and entitlements sections do.
func forbiddenCause(report *platform.Report, section platform.Section) string {
	if section.HTTPStatus != http.StatusForbidden {
		return ""
	}

	message := strings.ToLower(section.Message)

	switch {
	case strings.Contains(message, "seat license"):
		return causeSeat
	case strings.Contains(message, "feature flag"):
		return causeFlag
	case seatMissing(report):
		return causeSeat
	case flagOff(report):
		return causeFlag
	}

	return ""
}

// seatMissing reports whether the caller lacks the builder seat. A seat is
// listed only when the install enforces it.
func seatMissing(report *platform.Report) bool {
	seats, ok := report.Sections[platform.SectionSeats].Data.(platform.Seats)
	if !ok {
		return false
	}

	held, listed := seats.SeatLicenses[builderSeat]

	return listed && !held
}

// flagOff reports whether the containers flag is false for the caller.
func flagOff(report *platform.Report) bool {
	flags, ok := report.Sections[platform.SectionEntitlements].Data.(map[string]bool)
	if !ok {
		return false
	}

	value, known := flags[platform.EntitlementWorkloadAPIContainers]

	return known && !value
}
