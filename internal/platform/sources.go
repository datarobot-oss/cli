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
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// entitlementNames are the flags a Custom Application deploy depends on. The
// route rejects a whole batch for one unknown name, so entitlementsFor falls
// back to single names.
var entitlementNames = []string{
	"ENABLE_CUSTOM_APP_WORKLOAD_API_BACKEND",
	"ENABLE_WORKLOAD_API_CONTAINERS",
	"DISABLE_CUSTOM_TEMPLATES",
}

// entitlementBatchSize is the most names one evaluate call takes. A larger
// batch failed with a 500 on staging.
const entitlementBatchSize = 20

// listLimit is the page size asked of a paged route. A route may cap it lower;
// listAll follows the next cursor either way.
const listLimit = "100"

// publicConfig is the part of the unauthenticated /config route the report
// reads. The route is UI-shaped and unversioned, so a key it lacks stays unset.
type publicConfig struct {
	ReleaseVersion                 string `json:"RELEASE_VERSION"`
	ExternalWebServerURL           string `json:"EXTERNAL_WEB_SERVER_URL"`
	IsEnterprise                   *bool  `json:"IS_ENTERPRISE"`
	CustomAppDefaultResourceBundle string `json:"CUSTOM_APP_DEFAULT_RESOURCE_BUNDLE"`
}

func readConfig(ctx context.Context) (publicConfig, error) {
	var cfg publicConfig

	target, err := publicURL("/config")
	if err != nil {
		return cfg, err
	}

	err = call(ctx, http.MethodGet, target, false, nil, &cfg)

	return cfg, err
}

func readAPIVersion(ctx context.Context) (string, error) {
	var version struct {
		VersionString string `json:"versionString"`
	}

	target, err := apiURL("/version/", nil)
	if err != nil {
		return "", err
	}

	if err = call(ctx, http.MethodGet, target, true, nil, &version); err != nil {
		return "", err
	}

	return version.VersionString, nil
}

// sectionOf turns a read into a section: the data when it succeeded, the
// reason when it did not.
func sectionOf(data any, err error) Section {
	if err != nil {
		return Section{Status: StatusUnavailable, Message: reason(err)}
	}

	return Section{Status: StatusOK, Data: data}
}

func installSection(cfg publicConfig, err error) Section {
	return sectionOf(Install{
		IsEnterprise:             cfg.IsEnterprise,
		DefaultAppResourceBundle: cfg.CustomAppDefaultResourceBundle,
	}, err)
}

func seatsSection(ctx context.Context) Section {
	var info struct {
		SeatLicenses map[string]bool `json:"seatLicenses"`
	}

	target, err := apiURL("/account/info/", nil)
	if err == nil {
		err = call(ctx, http.MethodGet, target, true, nil, &info)
	}

	// A JSON null or a missing map must reach the report as {}, not null.
	seats := Seats{SeatLicenses: map[string]bool{}}
	for name, access := range info.SeatLicenses {
		seats.SeatLicenses[name] = access
	}

	return sectionOf(seats, err)
}

func executionEnvironmentsSection(ctx context.Context) Section {
	type environment struct {
		ID                      string   `json:"id"`
		Name                    string   `json:"name"`
		ProgrammingLanguage     string   `json:"programmingLanguage"`
		UseCases                []string `json:"useCases"`
		LatestSuccessfulVersion any      `json:"latestSuccessfulVersion"`
		LatestVersion           *struct {
			BuildStatus string `json:"buildStatus"`
		} `json:"latestVersion"`
	}

	target, err := apiURL("/executionEnvironments/", url.Values{"limit": {listLimit}})
	if err != nil {
		return sectionOf(nil, err)
	}

	raw, err := listAll[environment](ctx, target)
	if err != nil {
		return sectionOf(nil, err)
	}

	items := make([]ExecutionEnvironment, 0, len(raw))

	for _, e := range raw {
		item := ExecutionEnvironment{
			ID:                   e.ID,
			Name:                 e.Name,
			ProgrammingLanguage:  e.ProgrammingLanguage,
			UseCases:             append([]string{}, e.UseCases...),
			HasSuccessfulVersion: e.LatestSuccessfulVersion != nil,
		}

		if e.LatestVersion != nil {
			item.BuildStatus = e.LatestVersion.BuildStatus
		}

		items = append(items, item)
	}

	return sectionOf(ExecutionEnvironments{Items: items}, nil)
}

func resourceBundlesSection(ctx context.Context) Section {
	target, err := apiURL("/mlops/compute/bundles/", url.Values{"limit": {listLimit}})
	if err != nil {
		return sectionOf(nil, err)
	}

	items, err := listAll[ResourceBundle](ctx, target)
	if err != nil {
		return sectionOf(nil, err)
	}

	for i := range items {
		items[i].UseCases = append([]string{}, items[i].UseCases...)
	}

	if items == nil {
		items = []ResourceBundle{}
	}

	return sectionOf(ResourceBundles{Items: items}, nil)
}

func entitlementsSection(ctx context.Context) Section {
	return entitlementsFor(ctx, entitlementNames)
}

// entitlementsFor evaluates names in batches. One flag the install does not
// know costs only itself: the section is degraded and says which.
func entitlementsFor(ctx context.Context, names []string) Section {
	values := make(map[string]bool, len(names))
	failed := map[string]string{}

	for start := 0; start < len(names); start += entitlementBatchSize {
		got, missed := evaluateBatch(ctx, names[start:min(start+entitlementBatchSize, len(names))])

		maps.Copy(values, got)
		maps.Copy(failed, missed)
	}

	switch {
	case len(failed) == 0:
		return Section{Status: StatusOK, Data: values}
	case len(values) == 0:
		return Section{Status: StatusUnavailable, Message: failureMessage(failed)}
	default:
		return Section{Status: StatusDegraded, Message: failureMessage(failed), Data: values}
	}
}

// evaluateBatch asks for a whole batch. When the route answers with an HTTP
// error, it retries the batch name by name, because the route rejects every
// name for one it does not know. The second map holds the names that could not
// be read, with the reason.
func evaluateBatch(ctx context.Context, batch []string) (map[string]bool, map[string]string) {
	got, err := evaluate(ctx, batch)
	if err == nil {
		return got, nil
	}

	failed := map[string]string{}

	if isTransport(err) || len(batch) == 1 {
		for _, name := range batch {
			failed[name] = reason(err)
		}

		return nil, failed
	}

	values := make(map[string]bool, len(batch))

	for _, name := range batch {
		one, oneErr := evaluate(ctx, []string{name})
		if oneErr != nil {
			failed[name] = reason(oneErr)

			continue
		}

		values[name] = one[name]
	}

	return values, failed
}

// failureMessage lists the names that could not be read, in a stable order.
func failureMessage(failed map[string]string) string {
	names := make([]string, 0, len(failed))
	for name := range failed {
		names = append(names, name)
	}

	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %s", name, failed[name]))
	}

	return strings.Join(parts, "; ")
}

// evaluate asks the entitlements route for the values of names.
func evaluate(ctx context.Context, names []string) (map[string]bool, error) {
	type entitlement struct {
		Name string `json:"name"`
	}

	request := struct {
		Entitlements []entitlement `json:"entitlements"`
	}{}

	for _, name := range names {
		request.Entitlements = append(request.Entitlements, entitlement{Name: name})
	}

	var response struct {
		Entitlements []struct {
			Name  string `json:"name"`
			Value bool   `json:"value"`
		} `json:"entitlements"`
	}

	target, err := apiURL("/entitlements/evaluate/", nil)
	if err != nil {
		return nil, err
	}

	if err = call(ctx, http.MethodPost, target, true, request, &response); err != nil {
		return nil, err
	}

	values := make(map[string]bool, len(response.Entitlements))
	for _, e := range response.Entitlements {
		values[e.Name] = e.Value
	}

	return values, nil
}
