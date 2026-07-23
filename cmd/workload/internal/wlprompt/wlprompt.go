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

// Package wlprompt holds the small stdin prompt helpers shared by the workload
// porcelain commands (`config`, `up`). It mirrors the artifact-side dirprompt
// helpers, which live in an internal package the workload commands cannot import.
package wlprompt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/misc/reader"
)

// ResolveDir returns the project directory to operate on. A non-empty --dir
// wins; otherwise --yes (non-interactive) defaults to the current directory,
// and an interactive session is prompted with "." as the default.
func ResolveDir(dirFlag string, yes bool) (string, error) {
	if dirFlag != "" {
		return dirFlag, nil
	}

	if yes {
		return ".", nil
	}

	return AskWithDefault("Project directory", ".")
}

// Ask prompts for a required value on stderr and returns it trimmed, erroring
// on an empty answer.
func Ask(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)

	s, err := reader.ReadString()
	if err != nil {
		return "", err
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New(label + " is required")
	}

	return s, nil
}

// AskWithDefault prompts on stderr and returns defaultVal when the user just
// presses Enter.
func AskWithDefault(label, defaultVal string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultVal)

	s, err := reader.ReadString()
	if err != nil {
		return "", err
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal, nil
	}

	return s, nil
}
