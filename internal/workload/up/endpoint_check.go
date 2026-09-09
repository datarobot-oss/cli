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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/tui"
)

// endpointCheckTimeout bounds each GET the deploy ends with. The workload just
// reported running, so an app that is up answers quickly; the allowance is for
// one that is still booting, which the report then says.
const endpointCheckTimeout = 10 * time.Second

// endpointCheckAttempts and endpointCheckRetryDelay bound how long the check
// gives an application that has not finished booting.
//
// The GET is made when the platform promotes the new generation, which is
// minutes earlier than when it finishes collecting the old one, and a promotion
// says only that the container started: there is no warmup and no probe by
// default. Without a retry the line below would read "did not answer" on a
// healthy deploy of anything that takes a few seconds to listen, and a warning
// that is usually wrong is one people stop reading.
const endpointCheckAttempts = 3

// A var so tests do not sit through it. Nothing else writes it.
var endpointCheckRetryDelay = 5 * time.Second

// endpointProtonParam pins a workload endpoint request to one container
// generation. It is the DataRobot gateway's own parameter, resolved and
// consumed before the request is forwarded, so it never reaches the
// application.
const endpointProtonParam = "protonId"

type endpointCheckAuthMode string

const (
	endpointCheckAnonymous     endpointCheckAuthMode = "anonymous"
	endpointCheckAuthenticated endpointCheckAuthMode = "authenticated"
	workloadEndpointPathPrefix                       = config.DRAPIURLSuffix + "/endpoints/workloads/"
)

// checkEndpointFn performs the GET, swapped by tests. The request is
// anonymous for direct workload URLs and authenticated for the DataRobot public
// gateway URL. The gateway consumes the token before proxying, while a direct
// URL would carry the token into the user's container access logs.
var checkEndpointFn = func(rawURL string) (int, error) {
	client := drapi.NewHTTPClient(endpointCheckTimeout)

	// The endpoint's own answer, never a hop beyond it: a gateway that 302s
	// an anonymous GET to a login page would otherwise have this line report
	// the login page's 200 for a container serving nothing, which is the one
	// case the check exists to catch. A redirect is reported as the 3xx it is.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	authMode, err := endpointCheckAuthForURL(rawURL)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}

	if authMode == endpointCheckAuthenticated {
		if err = drapi.AuthorizeRequest(req); err != nil {
			return 0, err
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// endpointCheck is what the wait learned, and what the check needs in order to
// describe what it found honestly.
type endpointCheck struct {
	// ProtonID is the container generation the wait settled on, empty when the
	// platform named none or the wait never asked. The GET is pinned to it,
	// which is the only way to put the question to that generation: the
	// platform's router keeps a share of requests on the version being
	// replaced for minutes after the promotion.
	ProtonID string

	// Unconfirmed carries why the handover could not be read, and is empty
	// when it could. See workload.Serving.OnUnconfirmed.
	Unconfirmed string

	// PredecessorMayServe marks a run that replaced a generation and returned
	// at the promotion rather than at the drain, so the endpoint URL can still
	// hand back the previous version for a few minutes.
	PredecessorMayServe bool
}

// verifyEndpoint GETs the endpoint after the platform has said running, and
// reports what came back. With no readiness probe written by default,
// "running" only means the container started: nothing has checked that
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
func verifyEndpoint(result Result, check endpointCheck, report *reporter) {
	reportEndpointAnswer(result, check, report)
	sayPredecessorMayServe(check, report)
}

// reportEndpointAnswer makes the GET and says what came back.
func reportEndpointAnswer(result Result, check endpointCheck, report *reporter) {
	if result.Endpoint == "" {
		return
	}

	authMode, endpointErr := endpointCheckAuthForURL(result.Endpoint)
	if endpointErr != nil {
		report.say("  %s\n", tui.WarnStyle.Render("⚠ Skipping endpoint check: "+endpointErr.Error()))
		report.say("    %s\n", tui.HintStyle.Render(
			"The workload API returned an endpoint URL the CLI cannot GET."))

		return
	}

	// Pinned where the platform named a generation, so the answer below
	// describes the version this deploy put there rather than whichever one
	// the router's cache still points at. result.Endpoint itself is left
	// alone: it is what gets printed and what the JSON envelope carries, and a
	// pinned URL would be a link that dies when that generation is collected.
	target, pinned := pinnedEndpointURL(result.Endpoint, check.ProtonID)

	var (
		status int
		err    error
	)

	// Wrapped like every other wait in the package, because it can sit for
	// the whole timeout: a check that blocks for ten seconds printing nothing
	// reads as a hang. The closure returns nil whatever happened: the
	// check-marked line says the GET was made, and the lines below say what
	// it found; failing the run is the one thing this must never do.
	_ = report.run("Checking the endpoint", func() error {
		status, err = checkEndpointWithRetry(target)

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

	report.say("  %s\n", tui.HintStyle.Render(endpointAnswerLine(string(authMode), status, pinned)))

	// Said only when the wait could not confirm the handover, and carrying the
	// reason, because they are not the same problem: an install without the
	// route is a platform fact, while a 403 is this token's own permissions.
	// The GET above describes whatever answers now, and where the handover went
	// unconfirmed "now" can be inside the window where the workload already
	// names the new artifact and the endpoint is still on the old one.
	if check.Unconfirmed != "" {
		report.say("    %s\n", tui.HintStyle.Render(
			"The deploy could not tell whether the previous version has stopped answering, because "+
				check.Unconfirmed+". The line above may describe it."))
	}
}

// endpointAnswerLine says which generation was asked as well as what it said.
//
// The distinction is the whole of what a pinned check buys: on an unpinned GET
// the platform decides which generation answers, and for some minutes after a
// promotion that can be the version this deploy just replaced.
func endpointAnswerLine(authMode string, status int, pinned bool) string {
	if pinned {
		return "Endpoint check: an " + authMode +
			" GET, pinned to the container generation this deploy promoted, answered HTTP " +
			httpStatusLabel(status) + "."
	}

	return "Endpoint check: an " + authMode + " GET answered HTTP " + httpStatusLabel(status) + "."
}

// sayPredecessorMayServe states the one thing a deploy that returns at the
// promotion owes the reader: the platform has not finished moving the route
// across, so the endpoint URL can still hand back the previous version.
//
// Printed whatever the GET found, and printed where there is no endpoint to
// GET at all, because it is a fact about the deploy rather than about the
// check.
func sayPredecessorMayServe(check endpointCheck, report *reporter) {
	if !check.PredecessorMayServe {
		return
	}

	report.say("  %s\n", tui.HintStyle.Render(
		"The version this deploy replaced may still answer some requests on the endpoint URL for about "+
			"five more minutes, while the platform moves the route across. "+
			"Pass --wait-for-drain to have the deploy wait that out."))
}

// pinnedEndpointURL is rawURL with the GET aimed at one container generation,
// and false where pinning does not apply.
//
// Only a DataRobot workload gateway URL is pinned, which is exactly the set
// that receives the token. Everything else is a URL whose meaning the CLI does
// not own: protonId on a direct service URL would pin nothing and would arrive
// in somebody's application as a query parameter it never asked for, and a
// workload path on another origin is not this gateway.
//
// The parameter is merged rather than appended, because an endpoint URL is
// allowed to carry a query of its own.
func pinnedEndpointURL(rawURL, protonID string) (string, bool) {
	if protonID == "" {
		return rawURL, false
	}

	authMode, err := endpointCheckAuthForURL(rawURL)
	if err != nil || authMode != endpointCheckAuthenticated {
		return rawURL, false
	}

	endpointURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}

	query := endpointURL.Query()
	query.Set(endpointProtonParam, protonID)
	endpointURL.RawQuery = query.Encode()

	return endpointURL.String(), true
}

// checkEndpointWithRetry GETs rawURL, giving a container that has not finished
// booting a few seconds to start answering.
func checkEndpointWithRetry(rawURL string) (int, error) {
	var (
		status int
		err    error
	)

	for attempt := 1; ; attempt++ {
		status, err = checkEndpointFn(rawURL)
		if attempt >= endpointCheckAttempts || !endpointStillComingUp(status, err) {
			return status, err
		}

		time.Sleep(endpointCheckRetryDelay)
	}
}

// endpointStillComingUp reports whether the answer is one a container that is
// still starting gives, rather than one the application meant.
//
// A status code is the application's answer and is never retried, whatever it
// is: a 404 at / is how a healthy API-only framework answers, and asking again
// would only delay saying so. The exceptions are the three the gateway returns
// for itself when there is nothing behind it yet.
func endpointStillComingUp(status int, err error) bool {
	if err != nil {
		return true
	}

	return status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

// endpointCheckAuthForURL keeps the credential-safety decision next to the
// workload endpoint policy: only DataRobot public workload ingress URLs receive
// the CLI token, because direct service URLs can be owned by user containers.
func endpointCheckAuthForURL(rawURL string) (endpointCheckAuthMode, error) {
	endpointURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse endpoint URL %q: %w", rawURL, err)
	}

	if endpointURL.Scheme == "" || endpointURL.Host == "" {
		return "", fmt.Errorf("endpoint URL %q is not absolute", rawURL)
	}

	if !strings.HasPrefix(endpointURL.EscapedPath(), workloadEndpointPathPrefix) {
		return endpointCheckAnonymous, nil
	}

	matches, err := drapi.URLMatchesConfiguredBase(rawURL)
	if err != nil || !matches {
		// Public workload paths only receive credentials when they are on the
		// configured DataRobot origin. Log the anonymous fallback so debug users
		// can distinguish a gateway 401 from an intentional token-withheld check.
		log.Debug("endpoint check: withholding token because endpoint URL does not match configured base",
			"endpoint", rawURL,
			"configured_base", config.GetBaseURL(),
			"error", err)

		return endpointCheckAnonymous, nil
	}

	return endpointCheckAuthenticated, nil
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
