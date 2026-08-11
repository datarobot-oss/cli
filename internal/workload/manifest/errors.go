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

package manifest

import (
	"fmt"
	"strings"
)

// FieldError is one finding against the manifest, anchored to the line it
// came from. Path names YAML keys the way the file spells them, e.g.
// artifact.spec.containerGroups[0].containers[1].port.
type FieldError struct {
	Path string
	Line int
	Msg  string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("line %d: %s: %s", e.Line, e.Path, e.Msg)
}

// ValidationError collects every finding for one manifest: problems are
// reported in a single pass rather than stopping at the first.
type ValidationError struct {
	File   string
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Errors)+1)
	lines = append(lines, fmt.Sprintf("invalid manifest %s:", e.File))

	for _, fieldErr := range e.Errors {
		lines = append(lines, "  "+fieldErr.Error())
	}

	return strings.Join(lines, "\n")
}
