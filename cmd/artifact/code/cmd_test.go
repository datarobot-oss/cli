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

package code

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmd_RegistersDoctor(t *testing.T) {
	c := Cmd()

	names := make([]string, 0, len(c.Commands()))

	for _, sub := range c.Commands() {
		names = append(names, sub.Name())
	}

	assert.Contains(t, names, "doctor", "doctor is registered alongside init/sync/versions/checkout")
}
