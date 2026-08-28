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

package up

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/tui"
)

// endpointCheckTimeout bounds the one GET the deploy ends with. The workload
// just reported running, so an app that is up answers quickly; the allowance
// is for one that is still booting, which the report then says.
const endpointCheckTimeout = 10 * time.Second

// checkEndpointFn performs the GET, swapped by tests. The request is
// anonymous on purpose: the user's API token has no business in a container's
// access log, and a 401 or 403 proves something is serving as well as a 200
// does.
var checkEndpointFn = func(url string) (int, error) {
	client := drapi.NewHTTPClient(endpointCheckTimeout)

	// The endpoint's own answer, never a hop beyond it: a gateway that 302s
	// an anonymous GET to a login page would otherwise have this line report
	// the login page's 200 for a container serving nothing, which is the one
	// case the check exists to catch. A redirect is reported as the 3xx it is.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}

	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// verifyEndpoint GETs the endpoint once after the platform has said running,
// and reports what came back. With no readiness probe written by default,
// "running" only means the container started — nothing has checked that
// anything answers on the endpoint, so without this line a deploy serving
// nothing ends in a green tick and the discovery is left to whoever the URL
// was shared with.
//
// It reports and never fails the run, whatever comes back. The probe default
// was removed because a wrong guess about an app's routes kills a healthy
// deploy; failing the deploy on the same guess here would be the same
// mistake wearing a different line. The status code is stated rather than
// judged for exactly that reason: a 404 at / is how a healthy API-only
// framework answers, and only the person who wrote the app knows whether it
// is fine.
func verifyEndpoint(result Result, unconfirmed string, report *reporter) {
	if result.Endpoint == "" {
		return
	}

	var (
		status int
		err    error
	)

	// Wrapped like every other wait in the package, because it can sit for
	// the whole timeout: a check that blocks for ten seconds printing nothing
	// reads as a hang. The closure returns nil whatever happened — the
	// check-marked line says the GET was made, and the lines below say what
	// it found; failing the run is the one thing this must never do.
	_ = report.run("Checking the endpoint", func() error {
		status, err = checkEndpointFn(result.Endpoint)

		return nil
	})

	if err != nil {
		report.say("  %s\n", tui.WarnStyle.Render("⚠ The endpoint did not answer a GET: "+err.Error()))
		report.say("    %s\n", tui.HintStyle.Render(
			"The workload reports running, which only means the container started: it may still be booting, "+
				"listening on a different port than the manifest names, or serving nothing."))
		report.say("    %s\n", tui.HintStyle.Render(
			"Check 'dr workload logs "+result.WorkloadID+"', then GET the endpoint again."))

		return
	}

	report.say("  %s\n", tui.HintStyle.Render(
		"Endpoint check: an anonymous GET answered HTTP "+httpStatusLabel(status)+"."))

	// Said only when the wait could not confirm the handover, and carrying the
	// reason, because they are not the same problem: an install without the
	// route is a platform fact, while a 403 is this token's own permissions.
	// The GET above describes whatever answers now, and where the handover went
	// unconfirmed "now" can be inside the window where the workload already
	// names the new artifact and the endpoint is still on the old one.
	if unconfirmed != "" {
		report.say("    %s\n", tui.HintStyle.Render(
			"The deploy could not tell whether the previous version has stopped answering, because "+
				unconfirmed+". The line above may describe it."))
	}
}

// httpStatusLabel renders "404 Not Found" rather than a bare number, because
// the reader of this line is deciding whether their app is fine and the words
// carry more than the digits.
func httpStatusLabel(status int) string {
	if text := http.StatusText(status); text != "" {
		return fmt.Sprintf("%d %s", status, text)
	}

	return strconv.Itoa(status)
}
