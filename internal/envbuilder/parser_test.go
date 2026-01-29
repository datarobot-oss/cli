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

package envbuilder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariablesFromLines(t *testing.T) {
	t.Run("Returns valid values", func(t *testing.T) {
		lines := []string{
			"NAME0=\"value0\"\n",
			"NAME1=\"value1\"\n",
			"#NAME2=\"value2\"\n",
			"DATAROBOT_ENDPOINT=\"\"\n",
			"DATAROBOT_API_TOKEN=\"\"\n",
		}
		contentsExpected := "NAME0=\"value0\"\nNAME1=\"value1\"\n#NAME2=\"value2\"\nDATAROBOT_ENDPOINT=\"\"\nDATAROBOT_API_TOKEN=\"\"\n"

		variables, contents := VariablesFromLines(lines)

		assert.Len(t, variables, 5)

		i := 0
		assert.Equal(t, "NAME0", variables[i].Name)
		assert.Equal(t, "value0", variables[i].Value)
		assert.False(t, variables[i].Commented)
		assert.False(t, variables[i].Secret)

		i++
		assert.Equal(t, "NAME1", variables[i].Name)
		assert.Equal(t, "value1", variables[i].Value)
		assert.False(t, variables[i].Commented)
		assert.False(t, variables[i].Secret)

		i++
		assert.Equal(t, "NAME2", variables[i].Name)
		assert.Equal(t, "value2", variables[i].Value)
		assert.True(t, variables[i].Commented)
		assert.False(t, variables[i].Secret)

		i++
		assert.Equal(t, "DATAROBOT_ENDPOINT", variables[i].Name)
		assert.Empty(t, variables[i].Value)
		assert.False(t, variables[i].Commented)
		assert.False(t, variables[i].Secret)

		i++
		assert.Equal(t, "DATAROBOT_API_TOKEN", variables[i].Name)
		assert.Empty(t, variables[i].Value)
		assert.False(t, variables[i].Commented)
		assert.True(t, variables[i].Secret)

		assert.Equal(t, contentsExpected, contents)
	})
}

func TestNewFromLine(t *testing.T) {
	t.Run("Returns valid values", func(t *testing.T) {
		unquotedValues := map[string]string{
			"NAME1": "value1",
			"NAME2": "value2",
		}

		value1 := NewFromLine("NAME1=\"value1\"\n", unquotedValues)

		assert.Equal(t, "value1", value1.Value)

		value2 := NewFromLine("NAME2=\"value2\"\n", unquotedValues)

		assert.Equal(t, "value2", value2.Value)
	})
}
