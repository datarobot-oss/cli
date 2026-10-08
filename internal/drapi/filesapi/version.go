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

package filesapi

import (
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
)

// The first API version whose stage upload takes isExecutable.
const (
	executableSinceMajor = 2
	executableSinceMinor = 49
)

type apiVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

// supportsExecutable reports whether the server's stage upload takes
// isExecutable, asked once per client. Its form validation is strict, so an
// older server would refuse the field; a version that cannot be read reads as
// no, which uploads the file without its bit, as before.
func (c *httpClient) supportsExecutable() bool {
	c.execOnce.Do(func() {
		requestURL, err := drapi.EndpointURL("/version/", nil)
		if err != nil {
			return
		}

		var v apiVersion

		if err := drapi.GetJSON(requestURL, "version", &v); err != nil {
			log.Debug("Could not read the API version; uploading without the executable bit", "error", err)

			return
		}

		c.execSupported = v.Major > executableSinceMajor ||
			(v.Major == executableSinceMajor && v.Minor >= executableSinceMinor)
	})

	return c.execSupported
}
