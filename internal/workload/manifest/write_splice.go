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
	"fmt"
	"os"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/datarobot/cli/internal/fsutil"
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
	old, replacement, ok := scalarSource(node, value)
	if !ok {
		return nil, false
	}

	start, ok := tokenOffset(data, node.Line, node.Column)
	if !ok {
		return nil, false
	}

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

// scalarSource is the source text node occupies and the text that value
// takes in the same style, or false when the splice does not model the node.
func scalarSource(node *yaml.Node, value string) (old, replacement string, ok bool) {
	if !spliceableNode(node) || !plainSafe(node.Value) || !plainSafe(value) {
		return "", "", false
	}

	switch node.Style {
	case 0:
		return node.Value, value, true
	case yaml.DoubleQuotedStyle:
		return `"` + node.Value + `"`, `"` + value + `"`, true
	case yaml.SingleQuotedStyle:
		return `'` + node.Value + `'`, `'` + value + `'`, true
	case yaml.TaggedStyle, yaml.LiteralStyle, yaml.FoldedStyle, yaml.FlowStyle:
		return "", "", false
	}

	return "", "", false
}

// spliceableNode is a positioned string scalar with no anchor or explicit tag.
func spliceableNode(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.ScalarNode || node.Anchor != "" || node.Line < 1 || node.Column < 1 {
		return false
	}

	return node.Tag == "" || node.Tag == "!!str"
}

// plainSafe reports whether s reads back as itself in every style the splice
// handles: no quoting, escapes, or characters YAML treats as structure. A
// lone "-" is still a sequence marker.
func plainSafe(s string) bool {
	if s == "" || s == "-" {
		return false
	}

	for _, r := range s {
		if !plainRune(r) {
			return false
		}
	}

	return true
}

func plainRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.'
}

// tokenOffset is the byte offset of the 1-based line and column. Column
// counts runes, so any multi-byte text before the token is stepped over.
func tokenOffset(data []byte, line, column int) (int, bool) {
	offset, ok := lineOffset(data, line)
	if !ok {
		return 0, false
	}

	for n := 1; n < column; n++ {
		if offset >= len(data) || data[offset] == '\n' {
			return 0, false
		}

		_, width := utf8.DecodeRune(data[offset:])
		offset += width
	}

	return offset, true
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

	// Same value, same bytes: nothing to write. An alias carries its anchor
	// name in Value, so only a scalar can be compared.
	if value.Kind == yaml.ScalarNode && value.Value == workloadID {
		return data, true
	}

	return spliceScalar(data, value, workloadID)
}

// spliceExistingWorkloadID replaces a binding already in the file and reports
// whether it did; false leaves the file untouched for the tree edit.
func spliceExistingWorkloadID(path, workloadID string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}

	_, root, err := soleDocument(data, path, "record the workload id", func(format string, args ...any) error {
		return fmt.Errorf(format, args...)
	})
	if err != nil {
		return false, nil
	}

	out, ok := spliceWorkloadID(data, root, workloadID)
	if !ok {
		return false, nil
	}

	if bytes.Equal(out, data) {
		return true, nil
	}

	return true, fsutil.AtomicWriteFile(path, out)
}
