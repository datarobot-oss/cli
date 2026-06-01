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

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmd_BasicMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "version", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

func TestCmd_IsGroupOnly(t *testing.T) {
	cmd := Cmd()

	assert.Nil(t, cmd.RunE, "version is a group command and should not have a RunE")
}

func TestCmd_RegistersAllVerbs(t *testing.T) {
	cmd := Cmd()

	want := map[string]bool{
		"get":  false,
		"list": false,
	}

	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}

	for verb, present := range want {
		assert.True(t, present, "missing subcommand: %s", verb)
	}
}
