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
	"net/http"
	"sync"
	"time"

	"github.com/datarobot/cli/internal/version"
)

// describeTimeout bounds the whole run, paging included.
const describeTimeout = 60 * time.Second

// now is the clock behind the report's timestamp, a seam for tests.
var now = time.Now

// Describe reads the install's public routes and returns the report.
//
// The version route goes first, because it is the cheapest call that proves the
// install answers and the credentials work. When it fails without an answer, or
// with a 401, every later call would fail the same way, so Describe returns an
// error instead of a report of failures. After that every source runs
// concurrently, and one that fails becomes an unavailable section while the
// others still report.
func Describe(ctx context.Context) (*Report, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()

	apiVersion, err := readAPIVersion(ctx)

	switch {
	case err != nil && isTransport(err):
		return nil, fmt.Errorf("cannot reach the install: %s", reason(err))
	case statusOf(err) == http.StatusUnauthorized:
		return nil, fmt.Errorf("the install rejected the credentials: %s", reason(err))
	}

	var (
		cfg                                publicConfig
		cfgErr                             error
		seats, entitlements, envs, bundles Section
		wg                                 sync.WaitGroup
	)

	for _, read := range []func(){
		func() { cfg, cfgErr = readConfig(ctx) },
		func() { seats = seatsSection(ctx) },
		func() { entitlements = entitlementsSection(ctx) },
		func() { envs = executionEnvironmentsSection(ctx) },
		func() { bundles = resourceBundlesSection(ctx) },
	} {
		wg.Add(1)

		go func() {
			defer wg.Done()

			read()
		}()
	}

	wg.Wait()

	return &Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   now().UTC().Format(time.RFC3339),
		Producer:      Producer{Name: version.CliName, Version: version.Version},
		Server: Server{
			Release:      cfg.ReleaseVersion,
			APIVersion:   apiVersion,
			CanonicalURL: cfg.ExternalWebServerURL,
		},
		Sections: map[string]Section{
			SectionInstall:               installSection(cfg, cfgErr),
			SectionSeats:                 seats,
			SectionEntitlements:          entitlements,
			SectionExecutionEnvironments: envs,
			SectionResourceBundles:       bundles,
		},
	}, nil
}
