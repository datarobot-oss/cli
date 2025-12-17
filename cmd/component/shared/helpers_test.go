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
			name: "boolean true",
			args: []string{"enabled=true"},
			expected: map[string]interface{}{
				"enabled": true,
			},
		},
		{
			name: "boolean false",
			args: []string{"enabled=false"},
			expected: map[string]interface{}{
				"enabled": false,
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
			name: "value with equals sign",
			args: []string{"formula=a=b+c"},
			expected: map[string]interface{}{
				"formula": "a=b+c",
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
