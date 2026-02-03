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

package shared

// NormalizeIndexURL ensures the URL ends with index.json
func NormalizeIndexURL(url string) string {
	if len(url) > 0 && url[len(url)-1] == '/' {
		return url + "index.json"
	}

	if len(url) > 5 && url[len(url)-5:] != ".json" {
		return url + "/index.json"
	}

	return url
}
