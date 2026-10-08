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

package auth

// Server-advertised IdP login: a DataRobot deployment that trusts an identity
// provider directly can say so at <host>/.well-known/oauth-protected-resource
// (RFC 9728 protected-resource metadata), so `dr auth login <url>` signs in at
// that IdP with no --issuer or --client-id.
//
// Opt-in on both sides, so hosts that never publish it keep the API-key flow:
// the document must be JSON, name this exact host as its resource, and carry a
// `datarobot_cli` block with a client id. A plain RFC 9728 document written for
// other clients (MCP) has no such block and is ignored.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/log"
)

const (
	protectedResourcePath = "/.well-known/oauth-protected-resource"
	// The probe runs on every login against hosts that may not serve it, so a
	// slow or silent host must not hold up the API-key flow for long.
	resourceDiscoveryTimeout = 3 * time.Second
)

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	DataRobotCLI         *struct {
		ClientID    string   `json:"client_id"`
		RedirectURI string   `json:"redirect_uri"`
		Scopes      []string `json:"scopes"`
	} `json:"datarobot_cli"`
}

// DiscoverOIDCFromResource asks datarobotHost which identity provider it
// trusts. ok is false when the host advertises nothing usable, which callers
// treat as "use the API-key flow"; the reason is logged at debug level only.
func DiscoverOIDCFromResource(ctx context.Context, datarobotHost string) (OIDCConfig, bool) {
	cfg, err := discoverOIDCFromResource(ctx, datarobotHost)
	if err != nil {
		log.Debugf("No IdP login advertised by %s: %v", datarobotHost, err)

		return OIDCConfig{}, false
	}

	return cfg, true
}

func discoverOIDCFromResource(ctx context.Context, datarobotHost string) (OIDCConfig, error) {
	host := strings.TrimRight(datarobotHost, "/")

	if err := checkDiscoverableHost(host); err != nil {
		return OIDCConfig{}, err
	}

	meta, err := fetchProtectedResource(ctx, host)
	if err != nil {
		return OIDCConfig{}, err
	}

	return meta.oidcConfig(host)
}

// checkDiscoverableHost allows https, and plain http only on loopback (local
// port-forwards): an unauthenticated channel must never name the IdP.
func checkDiscoverableHost(host string) error {
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not an absolute URL", host)
	}

	if u.Scheme == "https" || (u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return nil
	}

	return errors.New("not https")
}

func fetchProtectedResource(ctx context.Context, host string) (*protectedResourceMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, resourceDiscoveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+protectedResourcePath, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		// A redirect could point anywhere; the document must come from this host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, oidcMaxResponseBytes))
	if err != nil {
		return nil, err
	}

	var meta protectedResourceMetadata

	// An SPA catch-all answers 200 with HTML; that is "nothing advertised".
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, errors.New("not a JSON document")
	}

	return &meta, nil
}

// oidcConfig turns the document into login settings, or explains why not.
func (m *protectedResourceMetadata) oidcConfig(host string) (OIDCConfig, error) {
	// RFC 9728 section 3.3: the document must describe the resource it was fetched for.
	if strings.TrimRight(m.Resource, "/") != host {
		return OIDCConfig{}, fmt.Errorf("resource %q does not match %q", m.Resource, host)
	}

	if m.DataRobotCLI == nil || m.DataRobotCLI.ClientID == "" || len(m.AuthorizationServers) == 0 {
		return OIDCConfig{}, errors.New("no datarobot_cli client or authorization server")
	}

	cfg := OIDCConfig{
		Issuer:      m.AuthorizationServers[0],
		ClientID:    m.DataRobotCLI.ClientID,
		Scopes:      m.DataRobotCLI.Scopes,
		RedirectURI: m.DataRobotCLI.RedirectURI,
	}.WithDefaults()

	if err := cfg.Validate(); err != nil {
		return OIDCConfig{}, fmt.Errorf("advertised settings are unusable: %w", err)
	}

	return cfg, nil
}
