// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
