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

package dirprompt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/misc/reader"
)

type (
	PromptFunc          func(label, defaultVal string) (string, error)
	PromptNoDefaultFunc func(label string) (string, error)
)

func ResolveDir(dirFlag string, yes bool, prompt PromptFunc) (string, error) {
	if dirFlag != "" {
		return dirFlag, nil
	}

	if yes {
		return ".", nil
	}

	return prompt("Initialize directory", ".")
}

func ResolveArtifactID(args []string, yes bool, prompt PromptNoDefaultFunc) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}

	if yes {
		return "", errors.New("artifact ID is required when using --yes")
	}

	return prompt("Artifact ID")
}

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
