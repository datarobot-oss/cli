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

package apiclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorDetail(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want string
	}{
		"string detail":   {`{"detail":"boom"}`, "boom"},
		"array detail":    {`{"detail":[{"msg":"field required"}]}`, `[{"msg":"field required"}]`},
		"no detail field": {`{"message":"drflask says hi"}`, ""},
		"not json":        {"<html>504 Gateway Time-out</html>", ""},
		"empty":           {"", ""},
		"null detail":     {`{"detail":null}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, ErrorDetail([]byte(c.body)))
		})
	}
}

// A detail-bearing failure reads as the server's own sentence, with the
// endpoint kept: "Internal server error" identifies nothing without it.
func TestLiftDetail_RewrapsAnEnvelopedError(t *testing.T) {
	inner := &drapi.HTTPError{
		StatusCode: http.StatusUnprocessableEntity,
		URL:        "https://example.test/api/v2/artifacts/a-1/builds/",
		Body:       []byte(`{"detail":"Artifact not found"}`),
	}
	err := LiftDetail(fmt.Errorf("%w: body=%s", inner, inner.Body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Artifact not found")
	assert.NotContains(t, err.Error(), "body=", "the raw dump is replaced by the sentence")
	assert.Contains(t, err.Error(), "/api/v2/artifacts/a-1/builds/")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "callers keep their errors.As status checks")
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Equal(t, "Artifact not found", httpErr.Detail)
	assert.NotEmpty(t, httpErr.Body, "the raw bytes survive for callers that read them")
}

// Everything that is not an enveloped HTTP failure passes through untouched:
// the lift adds meaning where it knows the shape and must not disturb
// anything else.
func TestLiftDetail_PassesEverythingElseThrough(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.NoError(t, LiftDetail(nil))
	})

	t.Run("non-HTTP error", func(t *testing.T) {
		plain := errors.New("dial tcp: connection refused")
		assert.Same(t, plain, LiftDetail(plain))
	})

	t.Run("HTTP error without an envelope", func(t *testing.T) {
		proxy := &drapi.HTTPError{
			StatusCode: http.StatusGatewayTimeout,
			URL:        "https://example.test/api/v2/artifacts/a-1/builds/",
			Body:       []byte("<html>504 Gateway Time-out</html>"),
		}
		wrapped := fmt.Errorf("%w: body=%s", proxy, proxy.Body)

		assert.Same(t, wrapped, LiftDetail(wrapped), "a body drapi's caller cannot read stays as it came")
	})
}
