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

package tools

import "testing"

func TestSufficientVersionTrue(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "3.5.7", minimal: "3.5.7"},
		{installed: "3.5.9", minimal: "3.5.7"},
		{installed: "3.7.6", minimal: "3.5.7"},
		{installed: "5.4.6", minimal: "3.5.7"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != true {
			t.Errorf("for installed %s and minimal %s, expected sufficient", testCase.installed, testCase.minimal)
		}
	}
}

func TestSufficientVersionFalse(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "2.6.8", minimal: "3.5.7"},
		{installed: "3.4.8", minimal: "3.5.7"},
		{installed: "3.5.6", minimal: "3.5.7"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != false {
			t.Errorf("for installed %s and minimal %s, expected insufficient", testCase.installed, testCase.minimal)
		}
	}
}
