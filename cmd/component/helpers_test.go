// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"testing"
)

func TestParseDataArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: map[string]interface{}{},
			wantErr:  false,
		},
		{
			name: "string values",
			args: []string{"key1=value1", "key2=value2"},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			wantErr: false,
		},
		{
			name: "boolean true",
			args: []string{"use_feature=true"},
			expected: map[string]interface{}{
				"use_feature": true,
			},
			wantErr: false,
		},
		{
			name: "boolean false",
			args: []string{"use_feature=false"},
			expected: map[string]interface{}{
				"use_feature": false,
			},
			wantErr: false,
		},
		{
			name: "mixed types",
			args: []string{"name=test", "enabled=true", "disabled=false"},
			expected: map[string]interface{}{
				"name":     "test",
				"enabled":  true,
				"disabled": false,
			},
			wantErr: false,
		},
		{
			name: "path values",
			args: []string{"base_answers_file=.datarobot/answers/base.yml"},
			expected: map[string]interface{}{
				"base_answers_file": ".datarobot/answers/base.yml",
			},
			wantErr: false,
		},
		{
			name:     "invalid format - no equals",
			args:     []string{"invalid"},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - empty key",
			args:     []string{"=value"},
			expected: nil,
			wantErr:  true,
		},
		{
			name: "value with equals sign",
			args: []string{"url=https://example.com?key=value"},
			expected: map[string]interface{}{
				"url": "https://example.com?key=value",
			},
			wantErr: false,
		},
		{
			name: "numeric values",
			args: []string{"port=8080", "timeout=30.5"},
			expected: map[string]interface{}{
				"port":    "8080",
				"timeout": "30.5",
			},
			wantErr: false,
		},
		{
			name: "list syntax - yaml style",
			args: []string{"python_versions=[3.10, 3.11, 3.12]"},
			expected: map[string]interface{}{
				"python_versions": "[3.10, 3.11, 3.12]",
			},
			wantErr: false,
		},
		{
			name: "list syntax - string items",
			args: []string{"databases=[postgres, mysql, sqlite]"},
			expected: map[string]interface{}{
				"databases": "[postgres, mysql, sqlite]",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDataArgs(tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseDataArgs() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr {
				if len(result) != len(tt.expected) {
					t.Errorf("parseDataArgs() got %d items, expected %d", len(result), len(tt.expected))

					return
				}

				for key, expectedValue := range tt.expected {
					actualValue, ok := result[key]
					if !ok {
						t.Errorf("parseDataArgs() missing key %s", key)

						continue
					}

					if actualValue != expectedValue {
						t.Errorf("parseDataArgs() key %s = %v, expected %v", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}
