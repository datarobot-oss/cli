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

package wapi

import (
	"errors"
	"fmt"
)

// ErrAlreadyLinked is returned by Initialize when the project directory
// already has a state directory.
var ErrAlreadyLinked = errors.New("Project already linked: state directory exists.")

// ErrNotInitialized is returned by Load/Save/Append operations when the
// project directory has no state directory. CLI consumers are responsible
// for translating this into a user-facing hint about how to initialize.
var ErrNotInitialized = errors.New("State directory not found.")

// CorruptedError wraps a read, parse, or semantic validation failure for a
// specific file under the state directory. It carries the absolute path of the
// corrupted file so callers can include it in user-facing diagnostics.
type CorruptedError struct {
	Path string
	Err  error
}

func (e *CorruptedError) Error() string {
	return fmt.Sprintf("State file is corrupted at %s: %v", e.Path, e.Err)
}

func (e *CorruptedError) Unwrap() error {
	return e.Err
}
