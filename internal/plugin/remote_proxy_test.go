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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin download used to build a bare &http.Transport{}, whose Proxy field
// is nil. That bypassed the corporate forward proxy the rest of the CLI honours
// and failed on a direct DNS lookup of the download host, and it also dropped
// the TLS configuration tls.Apply installs for --ca-cert / -k.

func TestNewDownloadTransportInheritsProxyAndTLSFromDefault(t *testing.T) {
	base, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok, "http.DefaultTransport should be *http.Transport")

	transport := newDownloadTransport()

	require.NotNil(t, transport.Proxy, "download must resolve a proxy, not connect directly")
	assert.Equal(t,
		reflect.ValueOf(base.Proxy).Pointer(),
		reflect.ValueOf(transport.Proxy).Pointer(),
		"download should resolve proxies the same way as the rest of the CLI",
	)

	// tls.Apply swaps http.DefaultTransport to carry --ca-cert / -k, so cloning
	// it is what keeps those flags effective for plugin downloads too.
	assert.Equal(t, base.TLSClientConfig, transport.TLSClientConfig)

	assert.NotNil(t, transport.DialContext, "the fail-fast dial timeout must be kept")
}

func TestNewDownloadTransportKeepsCustomTLSConfig(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	custom := original.(*http.Transport).Clone()
	custom.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // mirrors tls.Apply(--skip-certificate-check)
	http.DefaultTransport = custom

	transport := newDownloadTransport()

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

	var proxiedURL string

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedURL = r.URL.String()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	routed := original.(*http.Transport).Clone()
	routed.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	http.DefaultTransport = routed

	// A host that cannot resolve: reaching it at all proves the proxy was used.
	path, err := downloadHTTP("http://plugins.invalid/codespace-0.5.5.tar.xz")
	require.NoError(t, err, "download must succeed through the proxy")

	t.Cleanup(func() { _ = os.Remove(path) })

	assert.Equal(t, "http://plugins.invalid/codespace-0.5.5.tar.xz", proxiedURL,
		"the proxy should receive the absolute download URL")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}
