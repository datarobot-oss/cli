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

package reader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNonInteractive(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"true":  true,
		"TRUE":  true,
		"True":  true,
		"1":     true,
		"yes":   true,
		"y":     true,
		"false": false,
		"0":     false,
		"no":    false,
		"foo":   false,
	}

	for value, want := range cases {
		t.Setenv(NonInteractiveEnv, value)
		assert.Equalf(t, want, IsNonInteractive(), "value=%q", value)
	}
}
