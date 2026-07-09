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

package llmgateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmd_Use(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "llm-gateway", cmd.Use)
}

func TestCmd_Aliases(t *testing.T) {
	cmd := Cmd()

	assert.Contains(t, cmd.Aliases, "llm")
	assert.Contains(t, cmd.Aliases, "llm-gateways")
}

func TestCmd_Subcommands(t *testing.T) {
	cmd := Cmd()

	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}

	assert.True(t, names["list"], "expected 'list' subcommand")
	assert.True(t, names["select"], "expected 'select' subcommand")
}

func TestCmd_GroupID(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "core", cmd.GroupID)
}
