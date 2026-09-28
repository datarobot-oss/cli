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

// Package usecase holds the parts of the DataRobot Use Case concept that CLI
// commands share, starting with parsing a Use Case id from user input.
package usecase

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ID is a Use Case id that has passed ParseID.
type ID string

// objectID matches the 24-character hexadecimal ObjectId Use Case ids use.
var objectID = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

// ParseID trims raw and checks that it is a Use Case id. It rejects a blank
// value and anything that is not a 24-character hexadecimal ObjectId, so a
// typo fails before any request is sent.
func ParseID(raw string) (ID, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", errors.New("the Use Case id must be non-blank")
	}

	if !objectID.MatchString(id) {
		return "", fmt.Errorf("%q is not a Use Case id: expected 24 hexadecimal characters", id)
	}

	return ID(id), nil
}
