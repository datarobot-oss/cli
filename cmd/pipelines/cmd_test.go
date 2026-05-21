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

package pipelines

import (
	"testing"

	"github.com/datarobot/cli/internal/features"
	"github.com/stretchr/testify/assert"
)

func TestCmd_BasicMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "pipeline", cmd.Use)
	assert.Equal(t, "core", cmd.GroupID)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

func TestCmd_FeatureGate(t *testing.T) {
	cmd := Cmd()

	gate, ok := cmd.Annotations[features.AnnotationKey]
	assert.True(t, ok, "expected feature-gate annotation to be set")
	assert.Equal(t, "pipelines", gate)
}

func TestCmd_NoSubcommandsYet(t *testing.T) {
	cmd := Cmd()

	assert.Empty(t, cmd.Commands(), "base branch registers no subcommands")
}
