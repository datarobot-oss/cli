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

package artifact

import (
	"testing"

	"github.com/datarobot/cli/internal/features"
	"github.com/stretchr/testify/assert"
)

func TestCmd_NotFeatureGated(t *testing.T) {
	cmd := Cmd()

	// The artifact root is generally available: it must carry no feature-gate
	// annotation, or cli.CommandAdder drops it from the root command tree
	// unless an env var is set. See TestArtifactCommandPresentByDefault in
	// cmd/root_test.go for the end-to-end guard.
	assert.NotContains(t, cmd.Annotations, features.AnnotationKey,
		"artifact is GA and must not carry a %q annotation", features.AnnotationKey)
}
