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

// Describe reads the install's routes and returns the report.
//
// The version route goes first, because it is the cheapest call that proves the
// install answers and the credentials work. When it fails without an answer, or
// with a 401, every later call would fail the same way, so Describe returns an
// error instead of a report of failures: it says apart a failure on this
// machine (no endpoint, no token) from one of the network. After that every source runs
// concurrently, and one that fails becomes an unavailable section while the
// others still report.
func Describe(ctx context.Context) (*Report, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()

	apiVersion, err := readAPIVersion(ctx)

	switch {
	case err != nil && isTransport(err):
		return nil, stopped{"cannot reach the install: " + reason(err), err}
	case statusOf(err) == http.StatusUnauthorized:
		return nil, stopped{"the install rejected the credentials: " + reason(err), err}
	case err != nil && statusOf(err) == 0:
		return nil, stopped{"cannot send the request: " + reason(err), err}
	}

	var cfg publicConfig

	reads := []struct {
		name string
		read func() Section
	}{
		{SectionInstall, func() Section {
			var cfgErr error

			cfg, cfgErr = readConfig(ctx)

			return installSection(cfg, cfgErr)
		}},
		{SectionSeats, func() Section { return seatsSection(ctx) }},
		{SectionEntitlements, func() Section { return entitlementsSection(ctx) }},
		{SectionExecutionEnvironments, func() Section { return executionEnvironmentsSection(ctx) }},
		{SectionResourceBundles, func() Section { return resourceBundlesSection(ctx) }},
		{SectionWorkloadAPI, func() Section { return workloadAPISection(ctx) }},
	}

	sections := make([]Section, len(reads))

	var wg sync.WaitGroup

	for i, source := range reads {
		wg.Add(1)

		go func() {
			defer wg.Done()

			sections[i] = guard(source.read)
		}()
	}

	wg.Wait()

	report := &Report{
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Producer:    Producer{Name: version.CliName, Version: version.Version},
		Server: Server{
			Release:      cfg.ReleaseVersion,
			APIVersion:   apiVersion,
			CanonicalURL: cfg.ExternalWebServerURL,
		},
		Sections: make(map[string]Section, len(reads)),
	}

	for i, source := range reads {
		report.Sections[source.name] = scoped(scopeOf[source.name], sections[i])
	}

	return report, nil
}

// scopeOf says who each section's answer is true for. The install's own config
// is the same for every caller; every other section depends on who asks.
var scopeOf = map[string]Scope{
	SectionInstall:               ScopePlatform,
	SectionSeats:                 ScopeCaller,
	SectionEntitlements:          ScopeCaller,
	SectionExecutionEnvironments: ScopeCaller,
	SectionResourceBundles:       ScopeCaller,
	SectionWorkloadAPI:           ScopeCaller,
}

// stopped is the error that ends a run before any section is read. Its message
// carries the one-line reason, and it unwraps to the cause so the JSON error
// envelope still finds the *drapi.HTTPError behind it.
type stopped struct {
	message string
	cause   error
}

func (e stopped) Error() string { return e.message }

func (e stopped) Unwrap() error { return e.cause }

// InternalErrorPrefix starts the message of a section that a recovered panic
// produced. Such a section has no HTTP status, like a network failure, so the
// doctor tells the two apart by this prefix.
const InternalErrorPrefix = "internal error: "

// guard runs read. When read panics, the result is an unavailable section, so
// a broken source costs only its own section.
func guard(read func() Section) (section Section) {
	defer func() {
		if r := recover(); r != nil {
			section = Section{Status: StatusUnavailable, Message: fmt.Sprintf("%s%v", InternalErrorPrefix, r)}
		}
	}()

	return read()
}

// scoped marks who the section's answer is true for, whether or not the read
// succeeded.
func scoped(scope Scope, section Section) Section {
	section.Scope = scope

	return section
}
