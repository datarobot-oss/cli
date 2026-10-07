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

package platform

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixClock(t *testing.T) {
	t.Helper()

	prev := now
	now = func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }

	t.Cleanup(func() { now = prev })
}

// Every recorded install must produce a report the schema accepts: that is the
// contract held against two releases.
func TestDescribe_ReportMatchesTheSchemaForEveryRecording(t *testing.T) {
	fixClock(t)

	recordings := map[string]Server{
		"sts-11.12.0":  {Release: "11.12.0", APIVersion: "2.48", CanonicalURL: "https://dr.example.com"},
		"saas-11.14.0": {Release: "11.14.0", APIVersion: "2.49", CanonicalURL: "https://staging.datarobot.com"},
		"sts-11.14.0":  {Release: "11.14.0", APIVersion: "2.49", CanonicalURL: "https://dr.example.com"},
	}

	for dir, wantServer := range recordings {
		t.Run(dir, func(t *testing.T) {
			newFakeInstall(t, dir)

			report, err := Describe(context.Background())

			require.NoError(t, err)
			assert.Equal(t, wantServer, report.Server)
			assert.Equal(t, "2026-10-06T20:00:00Z", report.GeneratedAt)
			assert.Equal(t, "dr", report.Producer.Name)

			for name, section := range report.Sections {
				assert.Equal(t, StatusOK, section.Status, name)
			}

			assert.Len(t, report.Sections, 5)
			require.NoError(t, validateJSON(t, compileSchema(t), renderJSON(t, *report)))
		})
	}
}

// The scope says who a section's answer is true for. The install's own config
// is the same for every caller; every other section depends on who asks.
func TestDescribe_EverySectionStatesWhoItsAnswerIsTrueFor(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	report, err := Describe(context.Background())
	require.NoError(t, err)

	want := map[string]Scope{
		SectionInstall:               ScopePlatform,
		SectionSeats:                 ScopeCaller,
		SectionEntitlements:          ScopeCaller,
		SectionExecutionEnvironments: ScopeCaller,
		SectionResourceBundles:       ScopeCaller,
	}

	for name, scope := range want {
		assert.Equal(t, scope, report.Sections[name].Scope, name)
	}
}

func TestDescribe_AnUnavailableSectionStillStatesItsScope(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/mlops/compute/bundles/", route{status: http.StatusForbidden, body: `{}`})

	report, err := Describe(context.Background())
	require.NoError(t, err)

	assert.Equal(t, StatusUnavailable, report.Sections[SectionResourceBundles].Status)
	assert.Equal(t, ScopeCaller, report.Sections[SectionResourceBundles].Scope)
}

func TestDescribe_ASourceThatFailsDoesNotStopTheOthers(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/mlops/compute/bundles/", route{status: http.StatusForbidden, body: `{"message":"no bundles feature"}`})

	report, err := Describe(context.Background())

	require.NoError(t, err)
	assert.Equal(t, StatusUnavailable, report.Sections[SectionResourceBundles].Status)
	assert.Equal(t, `HTTP 403: {"message":"no bundles feature"}`, report.Sections[SectionResourceBundles].Message)
	assert.Equal(t, StatusOK, report.Sections[SectionExecutionEnvironments].Status)
	assert.Equal(t, StatusOK, report.Sections[SectionSeats].Status)
	require.NoError(t, validateJSON(t, compileSchema(t), renderJSON(t, *report)))
}

func TestDescribe_ALostConfigLeavesTheReleaseOutAndKeepsTheRest(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /config", route{status: http.StatusNotFound, body: `not found`})

	report, err := Describe(context.Background())

	require.NoError(t, err)
	assert.Equal(t, Server{APIVersion: "2.48"}, report.Server)
	assert.Equal(t, StatusUnavailable, report.Sections[SectionInstall].Status)
	assert.Equal(t, StatusOK, report.Sections[SectionSeats].Status)
}

func TestDescribe_ARefusedVersionRouteStillReports(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/version/", route{status: http.StatusInternalServerError, body: `boom`})

	report, err := Describe(context.Background())

	require.NoError(t, err)
	assert.Empty(t, report.Server.APIVersion)
	assert.Equal(t, "11.12.0", report.Server.Release)
}

func TestDescribe_StopsWhenTheCredentialsAreRejected(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/version/", route{status: http.StatusUnauthorized, body: `{"message":"Invalid token"}`})

	report, err := Describe(context.Background())

	require.Error(t, err)
	assert.Nil(t, report)
	assert.Contains(t, err.Error(), "rejected the credentials")
	assert.Contains(t, err.Error(), "HTTP 401")
}

func TestDescribe_StopsWhenTheInstallIsUnreachable(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.srv.Close()

	report, err := Describe(context.Background())

	require.Error(t, err)
	assert.Nil(t, report)
	assert.Contains(t, err.Error(), "cannot reach the install")
}
