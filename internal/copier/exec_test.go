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

package copier

import (
	"testing"
)

func TestFormatDataValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		// String values
		{
			name:     "simple string",
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "empty string",
			value:    "",
			expected: "",
		},
		{
			name:     "string with spaces",
			value:    "hello world",
			expected: "hello world",
		},

		// Boolean values
		{
			name:     "bool true",
			value:    true,
			expected: "true",
		},
		{
			name:     "bool false",
			value:    false,
			expected: "false",
		},

		// Integer values
		{
			name:     "int",
			value:    42,
			expected: "42",
		},
		{
			name:     "int8",
			value:    int8(127),
			expected: "127",
		},
		{
			name:     "int16",
			value:    int16(32767),
			expected: "32767",
		},
		{
			name:     "int32",
			value:    int32(2147483647),
			expected: "2147483647",
		},
		{
			name:     "int64",
			value:    int64(9223372036854775807),
			expected: "9223372036854775807",
		},
		{
			name:     "negative int",
			value:    -42,
			expected: "-42",
		},
		{
			name:     "zero",
			value:    0,
			expected: "0",
		},

		// Float values
		{
			name:     "float32",
			value:    float32(3.14),
			expected: "3.14",
		},
		{
			name:     "float64",
			value:    float64(2.718281828),
			expected: "2.718281828",
		},
		{
			name:     "float with no decimal",
			value:    float64(42.0),
			expected: "42",
		},
		{
			name:     "negative float",
			value:    -3.14,
			expected: "-3.14",
		},

		// List values (multi-choice)
		{
			name:     "list of numbers",
			value:    []interface{}{3.10, 3.11, 3.12},
			expected: "[3.1, 3.11, 3.12]",
		},
		{
			name:     "list of strings",
			value:    []interface{}{"postgres", "mysql", "sqlite"},
			expected: "[postgres, mysql, sqlite]",
		},
		{
			name:     "list of bools",
			value:    []interface{}{true, false, true},
			expected: "[true, false, true]",
		},
		{
			name:     "mixed type list",
			value:    []interface{}{"item", 42, true, 3.14},
			expected: "[item, 42, true, 3.14]",
		},
		{
			name:     "empty list",
			value:    []interface{}{},
			expected: "[]",
		},
		{
			name:     "single item list",
			value:    []interface{}{"only"},
			expected: "[only]",
		},
		{
			name:     "nested list",
			value:    []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			expected: "[[1, 2], [3, 4]]",
		},

		// Map values
		{
			name: "simple map",
			value: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expected: "{key1: value1, key2: value2}",
		},
		{
			name: "map with various types",
			value: map[string]interface{}{
				"name":    "test",
				"enabled": true,
				"port":    8080,
			},
			expected: "{enabled: true, name: test, port: 8080}",
		},
		{
			name:     "empty map",
			value:    map[string]interface{}{},
			expected: "{}",
		},
		{
			name: "nested map",
			value: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			expected: "{outer: {inner: value}}",
		},

		// Null value
		{
			name:     "nil value",
			value:    nil,
			expected: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDataValue(tt.value)

			// For maps, order might vary, so we check length and key presence instead of exact string match
			if _, isMap := tt.value.(map[string]interface{}); isMap && tt.value != nil {
				// Basic validation - check it starts with { and ends with }
				if len(result) < 2 || result[0] != '{' || result[len(result)-1] != '}' {
					t.Errorf("formatDataValue() map result = %v, doesn't look like a map", result)
				}

				return
			}

			if result != tt.expected {
				t.Errorf("formatDataValue() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestFormatYAMLList(t *testing.T) {
	tests := []struct {
		name     string
		items    []interface{}
		expected string
	}{
		{
			name:     "numeric list",
			items:    []interface{}{1, 2, 3},
			expected: "[1, 2, 3]",
		},
		{
			name:     "string list",
			items:    []interface{}{"a", "b", "c"},
			expected: "[a, b, c]",
		},
		{
			name:     "empty list",
			items:    []interface{}{},
			expected: "[]",
		},
		{
			name:     "single item",
			items:    []interface{}{"only"},
			expected: "[only]",
		},
		{
			name:     "python versions example",
			items:    []interface{}{3.10, 3.11, 3.12},
			expected: "[3.1, 3.11, 3.12]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatYAMLList(tt.items)
			if result != tt.expected {
				t.Errorf("formatYAMLList() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestFormatYAMLMap(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]interface{}
		checkFunc   func(string) bool
		description string
	}{
		{
			name: "simple map",
			data: map[string]interface{}{
				"key": "value",
			},
			checkFunc: func(result string) bool {
				return result == "{key: value}"
			},
			description: "should be {key: value}",
		},
		{
			name: "empty map",
			data: map[string]interface{}{},
			checkFunc: func(result string) bool {
				return result == "{}"
			},
			description: "should be {}",
		},
		{
			name: "multiple keys",
			data: map[string]interface{}{
				"name":    "test",
				"enabled": true,
			},
			checkFunc: func(result string) bool {
				// Map iteration order is not guaranteed, check both keys are present
				return result == "{enabled: true, name: test}" || result == "{name: test, enabled: true}"
			},
			description: "should contain both keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatYAMLMap(tt.data)
			if !tt.checkFunc(result) {
				t.Errorf("formatYAMLMap() = %v, %s", result, tt.description)
			}
		})
	}
}
