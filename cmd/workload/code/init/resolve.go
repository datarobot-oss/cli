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

package initcmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/misc/reader"
)

func resolveDir(dirFlag string, yes, isTTY bool, prompt func(label, defaultVal string) (string, error)) (string, error) {
	if dirFlag != "" {
		return dirFlag, nil
	}

	if yes || !isTTY {
		return ".", nil
	}

	return prompt("Initialize directory", ".")
}

func resolveArtifactID(args []string, yes, isTTY bool, prompt func(label string) (string, error)) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}

	if yes {
		return "", errors.New("artifact ID is required when using --yes")
	}

	if !isTTY {
		return "", errors.New("artifact ID is required (no TTY for prompting)")
	}

	return prompt("Artifact ID")
}

// Prompts go to stderr so they don't pollute stdout when it's piped.
func ask(label string) (string, error) {
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

func askWithDefault(label, defaultVal string) (string, error) {
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
