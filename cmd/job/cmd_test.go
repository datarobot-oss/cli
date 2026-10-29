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

package job

import (
	"testing"

	"github.com/datarobot/cli/internal/features"
	"github.com/stretchr/testify/assert"
)

func TestTheJobCommandsAreGated(t *testing.T) {
	assert.Equal(t, "job", Cmd().Annotations[features.AnnotationKey])
}

func TestTheJobCommands(t *testing.T) {
	subs := Cmd().Commands()
	names := make([]string, 0, len(subs))

	for _, sub := range subs {
		names = append(names, sub.Name())
	}

	assert.ElementsMatch(t, []string{"create", "list", "get", "update", "delete", "run", "runs", "cancel", "logs"}, names)
}
