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

package clone

import (
	"os/exec"
	"strings"
)

func gitClone(repoURL, dir string) (string, error) {
	cmd := exec.Command("git", "clone", repoURL, dir)

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}

func gitOrigin(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")

	cmd.Dir = dir

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(stdout))
}

func gitPull(dir string) (string, error) {
	cmd := exec.Command("git", "pull")

	cmd.Dir = dir

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}
