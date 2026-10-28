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

package plugin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin download used to build a bare &http.Transport{}, whose Proxy field
// is nil. That bypassed the corporate forward proxy the rest of the CLI honours
// and failed on a direct DNS lookup of the download host, and it also dropped
// the TLS configuration tls.Apply installs for --ca-cert / -k.

// restoreDefaultTransport returns the current default transport and puts it back
// after the test, since these tests replace it process-wide.
func restoreDefaultTransport(t *testing.T) *http.Transport {
	t.Helper()

	original := http.DefaultTransport

	t.Cleanup(func() { http.DefaultTransport = original })

	base, ok := original.(*http.Transport)
	require.True(t, ok, "http.DefaultTransport should be *http.Transport")

	return base
}

func TestNewDownloadTransportResolvesProxiesLikeTheRestOfTheCLI(t *testing.T) {
	base := restoreDefaultTransport(t)

	// Asserted by behaviour rather than by comparing function pointers, which
	// reflect's own documentation says do not identify a func.
	sentinel, err := url.Parse("http://sentinel.example:8080")
	require.NoError(t, err)

	routed := base.Clone()
	routed.Proxy = func(*http.Request) (*url.URL, error) { return sentinel, nil }
	http.DefaultTransport = routed

	transport, err := newDownloadTransport()
	require.NoError(t, err)
	require.NotNil(t, transport.Proxy, "download must resolve a proxy, not connect directly")

	target, err := url.Parse("https://cli.datarobot.com/plugins/x.tar.xz")
	require.NoError(t, err)

	resolved, err := transport.Proxy(&http.Request{URL: target})
	require.NoError(t, err)
	assert.Equal(t, sentinel, resolved,
		"the download should go wherever the CLI's configured transport says")
}

func TestNewDownloadTransportKeepsTheDefaultTransportTimeouts(t *testing.T) {
	base := restoreDefaultTransport(t)

	transport, err := newDownloadTransport()
	require.NoError(t, err)

	// A bare &http.Transport{} leaves these at zero: no TLS handshake deadline,
	// and idle connections that are never reaped. Cloning carries them over.
	assert.Equal(t, base.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	assert.NotZero(t, transport.TLSHandshakeTimeout)
	assert.Equal(t, base.IdleConnTimeout, transport.IdleConnTimeout)
	assert.NotZero(t, transport.IdleConnTimeout)
	assert.Equal(t, base.ExpectContinueTimeout, transport.ExpectContinueTimeout)
}

func TestNewDownloadTransportKeepsCustomTLSConfig(t *testing.T) {
	base := restoreDefaultTransport(t)

	custom := base.Clone()
	custom.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // mirrors tls.Apply(--skip-certificate-check)
	http.DefaultTransport = custom

	transport, err := newDownloadTransport()
	require.NoError(t, err)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify,
		"a TLS-intercepting proxy needs the CLI's TLS settings to reach the download too")
}

// downloadHTTP must route through whatever proxy http.DefaultTransport resolves.
// Driving it through DefaultTransport rather than HTTP_PROXY keeps the test
// deterministic: net/http caches the proxy environment on first use, so setting
// the variable here would be ignored once another test has made a request.
func TestDownloadHTTPGoesThroughTheProxy(t *testing.T) {
	const body = "plugin-archive-bytes"

	// Handed over a channel rather than a shared variable: the handler runs on
	// the server's goroutine and the assertion on the test's.
	proxied := make(chan string, 1)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case proxied <- r.URL.String():
		default:
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	base := restoreDefaultTransport(t)

	routed := base.Clone()
	routed.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	http.DefaultTransport = routed

	// A host that cannot resolve: reaching it at all proves the proxy was used.
	path, err := downloadHTTP("http://plugins.invalid/codespace-0.5.5.tar.xz")
	require.NoError(t, err, "download must succeed through the proxy")

	t.Cleanup(func() { _ = os.Remove(path) })

	select {
	case got := <-proxied:
		assert.Equal(t, "http://plugins.invalid/codespace-0.5.5.tar.xz", got,
			"the proxy should receive the absolute download URL")
	default:
		t.Fatal("the proxy never received a request")
	}

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}
