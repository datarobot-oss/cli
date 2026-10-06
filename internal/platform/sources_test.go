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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// route answers one request. fn, when set, decides from the request body.
type route struct {
	status int
	body   string
	fn     func(body string) (int, string)
}

// fakeInstall serves recorded responses for the routes describe reads. A test
// replaces a route to simulate a failing source.
type fakeInstall struct {
	mu     sync.Mutex
	routes map[string]route
	posts  []string
	srv    *httptest.Server
}

// newFakeInstall loads the recorded responses in testdata/<dir> and points the
// CLI at the server. {{BASE}} in a recording becomes the server's address.
func newFakeInstall(t *testing.T, dir string) *fakeInstall {
	t.Helper()

	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("testdata", dir, name))
		require.NoError(t, err)

		return string(raw)
	}

	fake := &fakeInstall{routes: map[string]route{
		"GET /config":                                 {body: read("config.json")},
		"GET /api/v2/version/":                        {body: read("version.json")},
		"GET /api/v2/account/info/":                   {body: read("account_info.json")},
		"POST /api/v2/entitlements/evaluate/":         {body: read("entitlements.json")},
		"GET /api/v2/executionEnvironments/":          {body: read("environments.json")},
		"GET /api/v2/mlops/compute/bundles/":          {body: read("bundles_page1.json")},
		"GET /api/v2/mlops/compute/bundles/?offset=2": {body: read("bundles_page2.json")},
	}}

	fake.srv = useServer(t, fake.serve)

	return fake
}

func (f *fakeInstall) serve(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	if offset := r.URL.Query().Get("offset"); offset != "" {
		key += "?offset=" + offset
	}

	var body string

	if r.Method == http.MethodPost {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)

		f.mu.Lock()
		f.posts = append(f.posts, body)
		f.mu.Unlock()
	}

	f.mu.Lock()
	rt, ok := f.routes[key]
	f.mu.Unlock()

	if !ok {
		http.NotFound(w, r)

		return
	}

	status, payload := rt.status, rt.body
	if rt.fn != nil {
		status, payload = rt.fn(body)
	}

	if status == 0 {
		status = http.StatusOK
	}

	writeJSON(w, status, strings.ReplaceAll(payload, "{{BASE}}", f.srv.URL))
}

func (f *fakeInstall) set(key string, rt route) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.routes[key] = rt
}

func (f *fakeInstall) postBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.posts...)
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	return string(raw)
}

func TestReadConfig_PicksTheKeysItNames(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	cfg, err := readConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "11.12.0", cfg.ReleaseVersion)
	assert.Equal(t, "https://dr.example.com", cfg.ExternalWebServerURL)
	assert.Equal(t, "cpu.xlarge", cfg.CustomAppDefaultResourceBundle)
	require.NotNil(t, cfg.IsEnterprise)
	assert.True(t, *cfg.IsEnterprise)
}

func TestReadConfig_LeavesAMissingKeyUnset(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /config", route{body: `{"RELEASE_VERSION": "11.12.0"}`})

	cfg, err := readConfig(context.Background())

	require.NoError(t, err)
	assert.Nil(t, cfg.IsEnterprise)
	assert.Empty(t, cfg.CustomAppDefaultResourceBundle)
}

func TestReadConfig_ReportsAPageThatIsNotJSON(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /config", route{body: `<html>login</html>`})

	_, err := readConfig(context.Background())

	assert.Error(t, err)
}

func TestReadAPIVersion_ReturnsTheVersionString(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	version, err := readAPIVersion(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "2.48", version)
}

func TestReadAPIVersion_ReturnsTheStatusOfARejection(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/version/", route{status: http.StatusUnauthorized, body: `{"message":"Invalid token"}`})

	_, err := readAPIVersion(context.Background())

	assert.Equal(t, http.StatusUnauthorized, statusOf(err))
}

func TestSeatsSection_KeepsAnEmptyMapAsAnEmptyObject(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	section := seatsSection(context.Background())

	assert.Equal(t, StatusOK, section.Status)
	assert.JSONEq(t, `{"seatLicenses":{}}`, jsonOf(t, section.Data))
}

func TestSeatsSection_ReturnsEachLicenseWithItsAccess(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/account/info/", route{body: `{"seatLicenses":{"AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER":true,"NON_BUILDER_USER":false}}`})

	section := seatsSection(context.Background())

	assert.JSONEq(t,
		`{"seatLicenses":{"AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER":true,"NON_BUILDER_USER":false}}`,
		jsonOf(t, section.Data))
}

func TestSeatsSection_TreatsANullMapAsEmpty(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/account/info/", route{body: `{"seatLicenses":null}`})

	section := seatsSection(context.Background())

	assert.JSONEq(t, `{"seatLicenses":{}}`, jsonOf(t, section.Data))
}

func TestSeatsSection_IsUnavailableWithTheRouteBody(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/account/info/", route{status: http.StatusForbidden, body: `{"message":"nope"}`})

	section := seatsSection(context.Background())

	assert.Equal(t, StatusUnavailable, section.Status)
	assert.Equal(t, `HTTP 403: {"message":"nope"}`, section.Message)
	assert.Nil(t, section.Data)
}

func TestExecutionEnvironmentsSection_KeepsEnvironmentsThatShareAName(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	section := executionEnvironmentsSection(context.Background())

	require.Equal(t, StatusOK, section.Status)

	items := section.Data.(ExecutionEnvironments).Items
	require.Len(t, items, 4)

	assert.Equal(t, ExecutionEnvironment{
		ID: "env-python", Name: "[DataRobot] Python 3.12 Applications Base", ProgrammingLanguage: "python",
		UseCases: []string{"customApplication"}, HasSuccessfulVersion: true, BuildStatus: "success",
	}, items[0])

	// Same name, never built: a reader must tell them apart by ID and build state.
	assert.Equal(t, ExecutionEnvironment{
		ID: "env-python-unbuilt", Name: "[DataRobot] Python 3.12 Applications Base", ProgrammingLanguage: "python",
		UseCases: []string{"customApplication"},
	}, items[2])
}

// A real single-tenant install lists two NodeJS 24 environments under one name,
// so a pipeline that picks by name cannot tell them apart.
func TestExecutionEnvironmentsSection_ARealInstallListsOneNameTwice(t *testing.T) {
	newFakeInstall(t, "sts-11.14.0")

	section := executionEnvironmentsSection(context.Background())

	require.Equal(t, StatusOK, section.Status)

	idsByName := map[string]map[string]bool{}

	for _, env := range section.Data.(ExecutionEnvironments).Items {
		if idsByName[env.Name] == nil {
			idsByName[env.Name] = map[string]bool{}
		}

		idsByName[env.Name][env.ID] = true
	}

	assert.Len(t, idsByName["[DataRobot] NodeJS 24 Applications Base"], 2)
}

func TestExecutionEnvironmentsSection_IsUnavailableWhenTheRouteRefuses(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/executionEnvironments/", route{status: http.StatusForbidden, body: `{"message":"no feature"}`})

	section := executionEnvironmentsSection(context.Background())

	assert.Equal(t, StatusUnavailable, section.Status)
	assert.Contains(t, section.Message, "HTTP 403")
}

func TestResourceBundlesSection_ReadsEveryPage(t *testing.T) {
	newFakeInstall(t, "sts-11.12.0")

	section := resourceBundlesSection(context.Background())

	require.Equal(t, StatusOK, section.Status)

	items := section.Data.(ResourceBundles).Items
	require.Len(t, items, 3)
	assert.Equal(t, ResourceBundle{ID: "cpu.xlarge", Name: "XL", UseCases: []string{"customApplication"}, MemoryBytes: 2147483648, CPUCount: 2}, items[1])
	assert.Equal(t, "gpu.small", items[2].ID)
}

func TestResourceBundlesSection_IsUnavailableWhenALaterPageFails(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /api/v2/mlops/compute/bundles/?offset=2", route{status: http.StatusInternalServerError, body: `boom`})

	section := resourceBundlesSection(context.Background())

	assert.Equal(t, StatusUnavailable, section.Status)
	assert.Equal(t, "HTTP 500: boom", section.Message)
}

// evaluate answers a batch with every name's value, and rejects the whole
// batch when one name is unknown to the install.
func evaluateRejecting(bad string) func(body string) (int, string) {
	return func(body string) (int, string) {
		var req struct {
			Entitlements []struct {
				Name string `json:"name"`
			} `json:"entitlements"`
		}

		_ = json.Unmarshal([]byte(body), &req)

		var out []string

		for _, e := range req.Entitlements {
			if e.Name == bad {
				return http.StatusUnprocessableEntity, `{"message":"unknown entitlement ` + bad + `"}`
			}

			out = append(out, fmt.Sprintf(`{"name":%q,"value":true}`, e.Name))
		}

		return http.StatusOK, `{"entitlements":[` + strings.Join(out, ",") + `]}`
	}
}

func TestEntitlementsFor_ReadsOneBatch(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")

	section := entitlementsFor(context.Background(), entitlementNames)

	assert.Equal(t, StatusOK, section.Status)
	assert.Equal(t, map[string]bool{
		"ENABLE_CUSTOM_APP_WORKLOAD_API_BACKEND": false,
		"ENABLE_WORKLOAD_API_CONTAINERS":         true,
		"DISABLE_CUSTOM_TEMPLATES":               false,
	}, section.Data)
	assert.Len(t, fake.postBodies(), 1)
	assert.JSONEq(t,
		`{"entitlements":[{"name":"ENABLE_CUSTOM_APP_WORKLOAD_API_BACKEND"},{"name":"ENABLE_WORKLOAD_API_CONTAINERS"},{"name":"DISABLE_CUSTOM_TEMPLATES"}]}`,
		fake.postBodies()[0])
}

func TestEntitlementsFor_SplitsALongListIntoBatches(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("POST /api/v2/entitlements/evaluate/", route{fn: evaluateRejecting("")})

	names := make([]string, 25)
	for i := range names {
		names[i] = fmt.Sprintf("ENABLE_FLAG_%02d", i)
	}

	section := entitlementsFor(context.Background(), names)

	assert.Equal(t, StatusOK, section.Status)
	assert.Len(t, section.Data, 25)

	posts := fake.postBodies()
	require.Len(t, posts, 2)
	assert.Equal(t, entitlementBatchSize, strings.Count(posts[0], `"name"`))
	assert.Equal(t, 5, strings.Count(posts[1], `"name"`))
}

func TestEntitlementsFor_AnUnknownNameDegradesInsteadOfLosingTheRest(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("POST /api/v2/entitlements/evaluate/", route{fn: evaluateRejecting("ENABLE_NOT_ON_THIS_INSTALL")})

	section := entitlementsFor(context.Background(), []string{"ENABLE_A", "ENABLE_NOT_ON_THIS_INSTALL", "ENABLE_B"})

	assert.Equal(t, StatusDegraded, section.Status)
	assert.Equal(t, map[string]bool{"ENABLE_A": true, "ENABLE_B": true}, section.Data)
	assert.Contains(t, section.Message, "ENABLE_NOT_ON_THIS_INSTALL")
	assert.Contains(t, section.Message, "HTTP 422")
}

func TestEntitlementsFor_IsUnavailableWhenNoNameCanBeRead(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("POST /api/v2/entitlements/evaluate/", route{status: http.StatusForbidden, body: `{"message":"no"}`})

	section := entitlementsFor(context.Background(), []string{"ENABLE_A", "ENABLE_B"})

	assert.Equal(t, StatusUnavailable, section.Status)
	assert.Nil(t, section.Data)
	assert.Contains(t, section.Message, "HTTP 403")
}

func TestInstallSection_IsUnavailableWhenConfigFails(t *testing.T) {
	section := installSection(publicConfig{}, assert.AnError)

	assert.Equal(t, StatusUnavailable, section.Status)
	assert.Equal(t, assert.AnError.Error(), section.Message)
}
