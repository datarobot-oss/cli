// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package shared

import (
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/copier"
)

type ListItem struct {
	current   bool
	checked   bool
	component copier.Answers
}

func (i ListItem) Title() string {
	return fmt.Sprintf("%s (%s)",
		i.component.ComponentDetails.Name,
		i.component.FileName,
	)
}

// TODO: Decide if we return something for description - don't think needed - it's really just us adhering to interface
func (i ListItem) Description() string { return "" }

func (i ListItem) FilterValue() string {
	return strings.ToLower(i.component.FileName)
}
