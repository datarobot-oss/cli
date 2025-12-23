// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package shared

import (
	"testing"
)

func TestParseDataArgs_ValidFormats(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]interface{}
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: map[string]interface{}{},
		},
		{
			name: "single string value",
			args: []string{"name=test"},
			expected: map[string]interface{}{
				"name": "test",
			},
		},
		{
			name: "string values",
			args: []string{"key1=value1", "key2=value2"},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "boolean true",
			args: []string{"use_feature=true"},
			expected: map[string]interface{}{
				"use_feature": true,
			},
		},
		{
			name: "boolean false",
			args: []string{"use_feature=false"},
			expected: map[string]interface{}{
				"use_feature": false,
			},
		},
		{
			name: "multiple values",
			args: []string{"name=test", "enabled=true", "port=8080"},
			expected: map[string]interface{}{
				"name":    "test",
				"enabled": true,
				"port":    "8080",
			},
		},
		{
			name: "path values",
			args: []string{"base_answers_file=.datarobot/answers/base.yml"},
			expected: map[string]interface{}{
				"base_answers_file": ".datarobot/answers/base.yml",
			},
		},
		{
			name: "value with equals sign",
			args: []string{"url=https://example.com?key=value"},
			expected: map[string]interface{}{
				"url": "https://example.com?key=value",
			},
		},
		{
			name: "numeric values",
			args: []string{"port=8080", "timeout=30.5"},
			expected: map[string]interface{}{
				"port":    "8080",
				"timeout": "30.5",
			},
		},
		{
			name: "list syntax - yaml style",
			args: []string{"python_versions=[3.10, 3.11, 3.12]"},
			expected: map[string]interface{}{
				"python_versions": "[3.10, 3.11, 3.12]",
			},
		},
		{
			name: "list syntax - string items",
			args: []string{"databases=[postgres, mysql, sqlite]"},
			expected: map[string]interface{}{
				"databases": "[postgres, mysql, sqlite]",
			},
		},
		{
			name: "whitespace trimmed",
			args: []string{" name = test "},
			expected: map[string]interface{}{
				"name": "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDataArgs(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("key %s: expected %v, got %v", key, expectedValue, result[key])
				}
			}
		})
	}
}

func TestParseDataArgs_InvalidFormats(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing equals sign",
			args: []string{"nametest"},
		},
		{
			name: "empty key",
			args: []string{"=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDataArgs(tt.args)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
