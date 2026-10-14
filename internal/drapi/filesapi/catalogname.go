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

// CatalogNameMaxLen is the Files API's ceiling on a catalog entry name,
// counted in characters rather than bytes because the server validates a
// decoded Python string. Overshooting it fails the whole create with a
// 422, so the name is clamped here instead.
const CatalogNameMaxLen = 255

// ClampCatalogName trims a name to what the Files API accepts. An empty
// name stays empty and callers leave it off the request, which is what
// puts the platform's own default on the entry.
//
// Exported so a caller assembling a name out of parts measures it the
// same way the server does, rather than keeping a second copy of the rule
// and its reasoning.
//
// Clamping rather than erroring: the name is a label on an entry whose
// contents are the point, and the artifact names it mirrors are allowed
// to be twenty times longer. Refusing the upload over a cosmetic field
// would trade a slightly shortened label for a failed sync.
func ClampCatalogName(name string) string {
	runes := []rune(name)
	if len(runes) <= CatalogNameMaxLen {
		return name
	}

	return string(runes[:CatalogNameMaxLen])
}
