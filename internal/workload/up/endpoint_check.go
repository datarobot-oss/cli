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
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
)

// endpointCheckTimeout bounds the one GET the deploy ends with. The workload
// just reported running, so an app that is up answers quickly; the allowance
// is for one that is still booting, which the report then says.
const endpointCheckTimeout = 10 * time.Second

type endpointCheckAuthMode string

const (
	endpointCheckAnonymous     endpointCheckAuthMode = "anonymous"
	endpointCheckAuthenticated endpointCheckAuthMode = "authenticated"
	workloadEndpointPathPrefix                       = config.DRAPIURLSuffix + "/endpoints/workloads/"
)

// keyProbePath is the route a probe asks, inside readinessProbe.
const keyProbePath = "path"

// servedArtifactFn reads the artifact the workload ended up serving, for the
// readiness probe it declares. Swapped by tests.
var servedArtifactFn = workload.GetArtifactDocument

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

// verifyEndpoint GETs the endpoint once after the platform has said running,
// and reports what came back. With no readiness probe written by default,
// "running" only means the container started — nothing has checked that
// anything answers on the endpoint, so without this line a deploy serving
// nothing ends in a green tick and the discovery is left to whoever the URL
// was shared with.
//
// The GET asks the readiness probe's path when the artifact now serving
// declares one, and / only when it declares none. The probe path is the route
// the app's author named as answering, and the one the platform's readiness
// gate asks inside the cluster before it reports running, so asking it here
// adds the hop that gate skips: the public gateway in front of the container.
// An app need not serve / at all.
//
// It reports and never fails the run, whatever comes back. The probe default
// was removed because a wrong guess about an app's routes kills a healthy
// deploy; failing the deploy on the same guess here would be the same
// mistake wearing a different line. The status code is stated rather than
// judged for exactly that reason: a 404 at / is how a healthy API-only
// framework answers, and only the person who wrote the app knows whether it
// is fine.
//
// It reports whether the check was interrupted, and nothing else: the deploy
// is not failed over it, but the caller has one irreversible step left and
// must not take it after the user asked to stop.
func verifyEndpoint(result Result, unconfirmed string, report *reporter) (interrupted bool) {
	if result.Endpoint == "" {
		return false
	}

	authMode, endpointErr := endpointCheckAuthForURL(result.Endpoint)
	if endpointErr != nil {
		report.say("  %s\n", tui.WarnStyle.Render("⚠ Skipping endpoint check: "+endpointErr.Error()))
		report.say("    %s\n", tui.HintStyle.Render(
			"The workload API returned an endpoint URL the CLI cannot GET."))

		return false
	}

	var (
		target, probePath string
		status            int
		err               error
	)

	// Wrapped like every other wait in the package, because it can sit for
	// the whole timeout: a check that blocks for ten seconds printing nothing
	// reads as a hang. The closure returns nil whatever the GET did — the
	// check-marked line says the GET was made, and the lines below say what
	// it found; failing the run over what this observes is the one thing it
	// must never do.
	phaseErr := report.run("Checking the endpoint", func() error {
		target, probePath = targetFor(result.Endpoint, servedArtifact(result.ArtifactID))
		status, err = checkEndpointFn(target)

		return nil
	})

	// The phase can still end without the GET having happened, because Ctrl-C
	// ends any phase. That is not a verdict on the deploy — the rollout
	// finished, which is why this runs at all — so the deploy stays a success
	// and only the check is dropped. Said out loud, because a deploy that
	// printed no endpoint line would otherwise look like one whose check
	// passed quietly.
	if Interrupted(phaseErr) {
		report.say("  %s\n", tui.WarnStyle.Render("⚠ Endpoint check skipped: interrupted."))
		report.say("    %s\n", tui.HintStyle.Render(
			"The workload is running; nothing here was cancelled. GET "+result.Endpoint+" to check it yourself."))

		return true
	}

	if err != nil {
		report.say("  %s\n", tui.WarnStyle.Render("⚠ The endpoint did not answer a GET: "+err.Error()))
		report.say("    %s\n", tui.HintStyle.Render(
			"The workload reports running, which only means the container started: it may still be booting, "+
				"listening on a different port than the manifest names, or serving nothing."))
		report.say("    %s\n", tui.HintStyle.Render(
			"Check 'dr workload logs "+result.WorkloadID+"', then GET the endpoint again."))

		return false
	}

	// The mode of the URL actually sent, which a probe path changes only by
	// climbing out of the endpoint with "..".
	if mode, modeErr := endpointCheckAuthForURL(target); modeErr == nil {
		authMode = mode
	}

	asked := "/"
	if probePath != "" {
		asked = probePath + ", the readiness probe's path,"
	}

	report.say("  %s\n", tui.HintStyle.Render(
		"Endpoint check: an "+string(authMode)+" GET of "+asked+" answered HTTP "+httpStatusLabel(status)+"."))

	// Said only of /, which the check asks when it found no route named: an
	// app need not serve it, and a bare 404 under a green tick reads as a
	// broken deploy. A 404 on a probe path is a real finding and stays bare.
	if probePath == "" && status == http.StatusNotFound {
		report.say("    %s\n", tui.HintStyle.Render(
			"The check found no readiness probe path to ask, so it asked for /; "+
				"a 404 there only means the app serves nothing at /."))
	}

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

	return false
}

// servedArtifact reads the artifact the workload ended up serving, nil when
// there is none to read or the read fails. The check then asks /, which is
// what it asked before it read probes at all, so a failed read costs the
// better route and nothing else.
func servedArtifact(artifactID string) workload.Document {
	if artifactID == "" {
		return nil
	}

	doc, err := servedArtifactFn(artifactID)
	if err != nil {
		log.Debug("endpoint check: cannot read the served artifact, asking /",
			"artifact", artifactID,
			"error", err)

		return nil
	}

	return doc
}

// targetFor is what the check GETs, and the probe path that URL asks: the
// endpoint with the served artifact's readiness probe path under it, or the
// endpoint itself and "" when there is no such path.
func targetFor(endpoint string, served workload.Document) (string, string) {
	path := readinessPath(served)
	if path == "" {
		return endpoint, ""
	}

	target, err := probeURL(endpoint, path)
	if err != nil {
		return endpoint, ""
	}

	return target, path
}

// readinessPath is the readiness probe path of the doc's primary container,
// "" when it declares none. The probe's port, scheme and host are not read,
// because the platform's readiness gate does not read them either: it asks
// the path on the container's own port, which is the port the endpoint
// reaches.
func readinessPath(doc workload.Document) string {
	probe, _ := workload.PrimaryContainerInDocument(doc)[keyReadinessProbe].(map[string]any)
	path, _ := probe[keyProbePath].(string)

	return strings.TrimSpace(path)
}

// probeURL is the endpoint with path under it. A query in the probe path is
// split off first, since JoinPath would escape its "?" into the path, and the
// endpoint's own query, which a proton-pinned URL carries, is kept.
func probeURL(endpoint, path string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	route, query, _ := strings.Cut(path, "?")
	target := base.JoinPath(route)

	if query != "" {
		target.RawQuery = strings.TrimPrefix(target.RawQuery+"&"+query, "&")
	}

	return target.String(), nil
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
