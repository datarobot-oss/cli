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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/platform"
)

func reportOf(sections map[string]platform.Section) *platform.Report {
	return &platform.Report{Sections: sections}
}

func workloadAnswers() platform.Section {
	return platform.Section{Scope: platform.ScopeCaller, Status: platform.StatusOK, Data: platform.WorkloadAPI{}}
}

func workloadRefuses(status int, message string) platform.Section {
	return platform.Section{
		Scope:      platform.ScopeCaller,
		Status:     platform.StatusUnavailable,
		HTTPStatus: status,
		Message:    message,
	}
}

func seatsOf(licenses map[string]bool) platform.Section {
	return platform.Section{
		Scope:  platform.ScopeCaller,
		Status: platform.StatusOK,
		Data:   platform.Seats{SeatLicenses: licenses},
	}
}

func flagsOf(flags map[string]bool) platform.Section {
	return platform.Section{Scope: platform.ScopeCaller, Status: platform.StatusOK, Data: flags}
}

func TestWorkloadDeploy_Outcomes(t *testing.T) {
	tests := []struct {
		name       string
		sections   map[string]platform.Section
		wantStatus core.Status
		wantRemedy string
	}{
		{
			name:       "the Workload API answers the caller",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadAnswers()},
			wantStatus: core.StatusOK,
		},
		{
			name: "an install without the route is a warning that points at Custom Applications",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(404, `HTTP 404: {"message": "Not Found"}`),
				platform.SectionSeats:       seatsOf(map[string]bool{}),
				platform.SectionEntitlements: {
					Scope:   platform.ScopeCaller,
					Status:  platform.StatusDegraded,
					Message: "unknown names",
					Data:    map[string]bool{"DISABLE_CUSTOM_TEMPLATES": false},
				},
			},
			wantStatus: core.StatusWARN,
			wantRemedy: remedyCustomApplication,
		},
		{
			name:       "a switched off proxy fails",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(502, "Backend service is deactivated.")},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyEnableProxy,
		},
		{
			name: "a refusal with the builder seat set to false names the seat",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(403, "seat has not been granted"),
				platform.SectionSeats:       seatsOf(map[string]bool{builderSeat: false}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyGrantSeat,
		},
		{
			name: "a refusal with the container flag off names the flag",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI:  workloadRefuses(403, "flag is not enabled"),
				platform.SectionSeats:        seatsOf(map[string]bool{}),
				platform.SectionEntitlements: flagsOf(map[string]bool{platform.EntitlementWorkloadAPIContainers: false}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyEnableFlag,
		},
		{
			name: "a refusal whose cause the report does not show carries the message",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI:  workloadRefuses(403, "something else"),
				platform.SectionSeats:        seatsOf(map[string]bool{builderSeat: true}),
				platform.SectionEntitlements: flagsOf(map[string]bool{platform.EntitlementWorkloadAPIContainers: true}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyReadMessage,
		},
		{
			name: "the non-builder seat alone is not the cause of a refusal",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI:  workloadRefuses(403, "something else"),
				platform.SectionSeats:        seatsOf(map[string]bool{"NON_BUILDER_USER": true}),
				platform.SectionEntitlements: flagsOf(map[string]bool{platform.EntitlementWorkloadAPIContainers: true}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyReadMessage,
		},
		{
			name: "a refusal with both the seat and the flag at fault names the seat first",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI:  workloadRefuses(403, "refused"),
				platform.SectionSeats:        seatsOf(map[string]bool{builderSeat: false}),
				platform.SectionEntitlements: flagsOf(map[string]bool{platform.EntitlementWorkloadAPIContainers: false}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyGrantSeat,
		},
		{
			name: "a message that names the seat license gets the seat remedy whatever the seats section says",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(403, "Agentic, Predictive and Governance Seat License has not been granted to this user."),
				platform.SectionSeats:       seatsOf(map[string]bool{builderSeat: true}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyGrantSeat,
		},
		{
			name: "a Workload API message that names the seat license gets the seat remedy",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(403, "This feature requires the 'Agentic + Predictive + Governance Builder' seat license"),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyGrantSeat,
		},
		{
			name: "a message that names a feature flag gets the flag remedy whatever the entitlements say",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI:  workloadRefuses(403, "Required feature flag WORKLOAD_API_CONTAINERS is not enabled."),
				platform.SectionEntitlements: flagsOf(map[string]bool{platform.EntitlementWorkloadAPIContainers: true}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyEnableFlag,
		},
		{
			name: "a Workload API message that says the feature flags disable it gets the flag remedy",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(403, "Workload API is disabled in feature flags"),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyEnableFlag,
		},
		{
			name: "a message that names the flag wins over a seat that the seats section shows missing",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(403, "Required feature flag WORKLOAD_API_CONTAINERS is not enabled."),
				platform.SectionSeats:       seatsOf(map[string]bool{builderSeat: false}),
			},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyEnableFlag,
		},
		{
			name:       "a recovered panic is a warning to run the check again and not a network problem",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(0, "internal error: boom")},
			wantStatus: core.StatusWARN,
			wantRemedy: remedyInternalError,
		},
		{
			name:       "a refusal with no seats or entitlements in the report carries the message",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(403, "refused")},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyReadMessage,
		},
		{
			name:       "a rejected credential fails and points at the credentials",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(401, "bad key")},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyCheckCredentials,
		},
		{
			name:       "a throttled probe is a warning to run the check again",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(429, "slow down")},
			wantStatus: core.StatusWARN,
			wantRemedy: remedyRetry,
		},
		{
			name:       "an unavailable service fails with the message",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(503, "unavailable")},
			wantStatus: core.StatusFAIL,
			wantRemedy: remedyReadMessage,
		},
		{
			name:       "a route that could not be read is a warning to run the check again and check the network",
			sections:   map[string]platform.Section{platform.SectionWorkloadAPI: workloadRefuses(0, "timeout")},
			wantStatus: core.StatusWARN,
			wantRemedy: remedyCheckNetwork,
		},
		{
			name: "a throttled probe stays a warning whatever the seats say",
			sections: map[string]platform.Section{
				platform.SectionWorkloadAPI: workloadRefuses(429, "slow down"),
				platform.SectionSeats:       seatsOf(map[string]bool{builderSeat: false}),
			},
			wantStatus: core.StatusWARN,
			wantRemedy: remedyRetry,
		},
		{
			name:       "a report without the section is skipped",
			sections:   map[string]platform.Section{},
			wantStatus: core.StatusSKIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateWorkloadDeploy(reportOf(tt.sections))

			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantRemedy, got.Remedy)
			assert.NotEmpty(t, got.Summary)
			assert.False(t, got.Fixable)
		})
	}
}

func TestWorkloadDeploy_RefusalCarriesStatusAndMessage(t *testing.T) {
	got := evaluateWorkloadDeploy(reportOf(map[string]platform.Section{
		platform.SectionWorkloadAPI: workloadRefuses(403, "something else"),
	}))

	assert.Equal(t, "403", got.Details["httpStatus"])
	assert.Equal(t, "something else", got.Details["message"])
}

func TestWorkloadDeploy_ANotFoundSpeaksOfTheInstallNotTheRelease(t *testing.T) {
	got := evaluateWorkloadDeploy(reportOf(map[string]platform.Section{
		platform.SectionWorkloadAPI: workloadRefuses(404, "Not Found"),
	}))

	assert.Equal(t, "This install does not serve the Workload API.", got.Summary)
	assert.NotContains(t, got.Remedy, "release")
}

func TestChecks_IDsAreStableAndOrdered(t *testing.T) {
	checks := Checks(func(context.Context) (*platform.Report, error) { return reportOf(nil), nil })

	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.ID())
	}

	assert.Equal(t, []string{"platform.install.read", "platform.workload.deploy"}, ids)
}

func TestChecks_UnreadableInstallFailsTheReadAndSkipsTheRest(t *testing.T) {
	checks := Checks(func(context.Context) (*platform.Report, error) { return nil, errors.New("install unreachable") })

	results := core.NewRunner(checks...).Run(context.Background())

	require.Len(t, results, 2)
	assert.Equal(t, core.StatusFAIL, results[0].Status)
	assert.Contains(t, results[0].Summary, "install unreachable")
	assert.Equal(t, remedyCheckCredentials, results[0].Remedy)
	assert.Equal(t, core.StatusSKIP, results[1].Status)
}

func TestInstallRead_NamesTheRelease(t *testing.T) {
	checks := Checks(func(context.Context) (*platform.Report, error) {
		return &platform.Report{Server: platform.Server{Release: "11.15.0"}}, nil
	})

	got := checks[0].Run(context.Background())

	assert.Equal(t, core.StatusOK, got.Status)
	assert.Contains(t, got.Summary, "11.15.0")
}

func TestInstallRead_WithoutAReleaseStillSaysTheInstallAnswered(t *testing.T) {
	checks := Checks(func(context.Context) (*platform.Report, error) { return reportOf(nil), nil })

	got := checks[0].Run(context.Background())

	assert.Equal(t, core.StatusOK, got.Status)
	assert.Equal(t, "The install answered.", got.Summary)
}

func TestChecks_ReadTheInstallOncePerChecksValue(t *testing.T) {
	calls := 0
	checks := Checks(func(context.Context) (*platform.Report, error) {
		calls++

		return reportOf(map[string]platform.Section{platform.SectionWorkloadAPI: workloadAnswers()}), nil
	})

	runner := core.NewRunner(checks...)

	runner.Run(context.Background())
	results := runner.Run(context.Background())

	require.Len(t, results, 2)
	assert.Equal(t, core.StatusOK, results[1].Status)
	assert.Equal(t, 1, calls)
}
