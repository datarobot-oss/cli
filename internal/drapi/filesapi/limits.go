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

import "time"

// HTTP-level tunables for the upload and download paths.
//
// upload and download share a value today, but stay separate on purpose:
// the upload ceiling tracks request-body streaming time (zip + multipart
// stage), the download ceiling tracks response-body streaming time, and
// the two can drift apart as the backend's per-path limits change.
const (
	uploadHTTPTimeout   = 600 * time.Second
	downloadHTTPTimeout = 600 * time.Second

	// statusPollHTTPTimeout caps a single async-status poll. Each call
	// hits /status/<id>/ and returns immediately (200 with state, 303
	// when completed); the server does not block, so a short ceiling is
	// fine and prevents wedged sockets from stalling the engine loop.
	statusPollHTTPTimeout = 30 * time.Second
)
