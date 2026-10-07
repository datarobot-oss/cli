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

package drapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasStatus(t *testing.T) {
	notFound := &HTTPError{StatusCode: http.StatusNotFound}

	assert.True(t, HasStatus(notFound, http.StatusNotFound))
	assert.False(t, HasStatus(notFound, http.StatusConflict), "a different code does not match")

	// It unwraps, so a caller that wrapped the error with context still matches.
	wrapped := fmt.Errorf("deleting credential: %w", notFound)
	assert.True(t, HasStatus(wrapped, http.StatusNotFound))

	// A plain error, and nil, carry no status.
	assert.False(t, HasStatus(errors.New("boom"), http.StatusNotFound))
	assert.False(t, HasStatus(nil, http.StatusNotFound))
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(&HTTPError{StatusCode: http.StatusNotFound}))
	assert.False(t, IsNotFound(&HTTPError{StatusCode: http.StatusForbidden}))
	assert.False(t, IsNotFound(errors.New("boom")))
}
