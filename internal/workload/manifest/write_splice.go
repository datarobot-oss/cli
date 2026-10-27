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

package manifest

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// spliceScalar replaces the source text of one scalar node with value and
// returns the whole file, or false when the token cannot be located exactly.
//
// Re-encoding the tree is not byte-faithful: yaml.v3 hangs a comment block
// that follows the last item of a nested sequence on that item's last key and
// emits it two columns further right. Only a plain or quoted one-line scalar
// with no tag or anchor, whose bytes at the parsed position match, is spliced;
// anything else falls back to the tree edit.
func spliceScalar(data []byte, node *yaml.Node, value string) ([]byte, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Anchor != "" || node.Line < 1 || node.Column < 1 {
		return nil, false
	}

	if node.Tag != "" && node.Tag != "!!str" {
		return nil, false
	}

	var old, replacement string

	switch node.Style {
	case 0:
		if !plainSafe(node.Value) || !plainSafe(value) {
			return nil, false
		}

		old, replacement = node.Value, value
	case yaml.DoubleQuotedStyle:
		if !plainSafe(node.Value) || !plainSafe(value) {
			return nil, false
		}

		old, replacement = `"`+node.Value+`"`, `"`+value+`"`
	case yaml.SingleQuotedStyle:
		if !plainSafe(node.Value) || !plainSafe(value) {
			return nil, false
		}

		old, replacement = `'`+node.Value+`'`, `'`+value+`'`
	default:
		return nil, false
	}

	start, ok := lineOffset(data, node.Line)
	if !ok {
		return nil, false
	}

	start += node.Column - 1
	end := start + len(old)

	if end > len(data) || string(data[start:end]) != old {
		return nil, false
	}

	out := make([]byte, 0, len(data)-len(old)+len(replacement))
	out = append(out, data[:start]...)
	out = append(out, replacement...)
	out = append(out, data[end:]...)

	return out, true
}

// plainSafe reports whether s reads back as itself in every style the splice
// handles: no quoting, escapes, or characters YAML treats as structure.
func plainSafe(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}

	return true
}

// lineOffset is the byte offset at which the 1-based line begins.
func lineOffset(data []byte, line int) (int, bool) {
	offset := 0

	for n := 1; n < line; n++ {
		i := bytes.IndexByte(data[offset:], '\n')
		if i < 0 {
			return 0, false
		}

		offset += i + 1
	}

	return offset, true
}

// workloadIDValue is the value node of the binding, or nil when the key is
// absent.
func workloadIDValue(root *yaml.Node) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == keyWorkloadID {
			return root.Content[i+1]
		}
	}

	return nil
}

// spliceWorkloadID rewrites an existing binding in place and reports whether
// it could. A file with no binding, or one it cannot splice, reports false
// and is left for the tree edit.
func spliceWorkloadID(data []byte, root *yaml.Node, workloadID string) ([]byte, bool) {
	value := workloadIDValue(root)
	if value == nil {
		return nil, false
	}

	// Same value, same bytes: nothing to write.
	if strings.TrimSpace(value.Value) == workloadID {
		return data, true
	}

	return spliceScalar(data, value, workloadID)
}
