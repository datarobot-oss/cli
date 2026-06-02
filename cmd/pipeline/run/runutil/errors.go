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

package runutil

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/tui"
)

// HandleRunNotFoundError converts a 404 into a friendly informational message
// (returns nil) so the caller does not surface a raw HTTP error for a no-op.
func HandleRunNotFoundError(err error, runID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No run found with id: " + runID))

		return nil
	}

	return err
}
