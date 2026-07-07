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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
)

// Options configures TLS behavior for outbound HTTP requests.
type Options struct {
	SkipVerify bool
	CACertPath string
}

// Apply replaces http.DefaultTransport with a transport configured per opts.
// Must be called before any HTTP requests are made.
func Apply(opts Options) error {
	base, ok := http.DefaultTransport.(*http.Transport)

	if !ok {
		return errors.New("http.DefaultTransport is not *http.Transport")
	}

	t := base.Clone()

	if opts.SkipVerify {
		t.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // user-requested via --skip-certificate-check
		}

		http.DefaultTransport = t

		return nil
	}

	if opts.CACertPath == "" {
		return nil
	}

	pool, err := loadCACert(opts.CACertPath)
	if err != nil {
		return err
	}

	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	}

	t.TLSClientConfig.RootCAs = pool

	http.DefaultTransport = t

	return nil
}

// PropagateEnv sets TLS-related env vars so child processes (e.g. Node.js/Bun
// plugins) inherit the same TLS configuration as the current process.
// Call after Apply.
func PropagateEnv(opts Options) error {
	if opts.SkipVerify {
		return os.Setenv("NODE_TLS_REJECT_UNAUTHORIZED", "0")
	}

	if opts.CACertPath != "" {
		if err := os.Setenv("NODE_EXTRA_CA_CERTS", opts.CACertPath); err != nil {
			return err
		}

		return os.Setenv("SSL_CERT_FILE", opts.CACertPath)
	}

	return nil
}

func loadCACert(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert %q: %w", path, err)
	}

	pool := x509.NewCertPool()

	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no valid certificates found in %q", path)
	}

	return pool, nil
}
