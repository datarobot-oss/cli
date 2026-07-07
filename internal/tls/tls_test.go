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

package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreDefaultTransport saves http.DefaultTransport and restores it after
// the test. Must be called before any Apply() that may mutate the transport.
func restoreDefaultTransport(t *testing.T) {
	t.Helper()

	orig := http.DefaultTransport

	t.Cleanup(func() { http.DefaultTransport = orig })
}

// generateTestCAPEM writes a minimal self-signed CA certificate as a PEM file
// to a temp path and returns the path. The file is removed after the test.
func generateTestCAPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	f, err := os.CreateTemp(t.TempDir(), "test-ca-*.pem")
	require.NoError(t, err)

	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	require.NoError(t, f.Close())

	return f.Name()
}

func TestApply_NoOp(t *testing.T) {
	restoreDefaultTransport(t)

	orig := http.DefaultTransport

	require.NoError(t, Apply(Options{}))

	assert.Same(t, orig, http.DefaultTransport, "empty Options must not replace DefaultTransport")
}

func TestApply_SkipVerify(t *testing.T) {
	restoreDefaultTransport(t)

	require.NoError(t, Apply(Options{SkipVerify: true}))

	transport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestApply_SkipVerify_PreservesTransportDefaults(t *testing.T) {
	restoreDefaultTransport(t)

	require.NoError(t, Apply(Options{SkipVerify: true}))

	transport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)

	// Clone() preserves proxy and dial settings; verify the transport is still
	// a non-zero value (not an empty struct).
	assert.NotNil(t, transport.DialContext, "DialContext should be preserved from default transport")
}

func TestApply_CACert_ValidFile(t *testing.T) {
	restoreDefaultTransport(t)

	caPath := generateTestCAPEM(t)

	require.NoError(t, Apply(Options{CACertPath: caPath}))

	transport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs, "RootCAs pool should be populated")
}

func TestApply_CACert_FileNotFound(t *testing.T) {
	restoreDefaultTransport(t)

	err := Apply(Options{CACertPath: "/nonexistent/path/ca.pem"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading CA cert")
}

func TestApply_CACert_InvalidPEM(t *testing.T) {
	restoreDefaultTransport(t)

	f, err := os.CreateTemp(t.TempDir(), "bad-ca-*.pem")
	require.NoError(t, err)

	_, err = f.WriteString("not a pem block\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	err = Apply(Options{CACertPath: f.Name()})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid certificates found")
}

func TestApply_BothSkipVerifyAndCACert_SkipVerifyWins(t *testing.T) {
	restoreDefaultTransport(t)

	caPath := generateTestCAPEM(t)

	require.NoError(t, Apply(Options{SkipVerify: true, CACertPath: caPath}))

	transport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Nil(t, transport.TLSClientConfig.RootCAs, "RootCAs should not be set when SkipVerify wins")
}
