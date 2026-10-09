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

package rewriteconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/copier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const communityRepoURL = "https://github.com/datarobot-community/af-component-agent.git"

func setupAnswersDir(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".datarobot", "answers"), 0o755))
}

func writeAnswersFile(t *testing.T, dir, name, srcPath string) string {
	t.Helper()

	content := "_src_path: " + srcPath + "\nsome_answer: value\n"
	path := filepath.Join(dir, ".datarobot", "answers", name)

	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

func readSrcPath(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var result struct {
		SrcPath string `yaml:"_src_path"`
	}

	require.NoError(t, yaml.Unmarshal(data, &result))

	return result.SrcPath
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	orig, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))

	t.Cleanup(func() {
		require.NoError(t, os.Chdir(orig))
	})
}

func TestRunE_NoEnvVar(t *testing.T) {
	t.Setenv(copier.EnvApplicationTemplateGitBaseURL, "")

	dir := t.TempDir()

	setupAnswersDir(t, dir)
	path := writeAnswersFile(t, dir, "component.yml", communityRepoURL)
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	chdir(t, dir)

	require.NoError(t, RunE(nil, nil))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(after), "file should be unchanged when env var is not set")
}

func TestRunE_EnvVarSet_RewritesFiles(t *testing.T) {
	const customBase = "https://internal.example.com/mirror"

	t.Setenv(copier.EnvApplicationTemplateGitBaseURL, customBase)

	dir := t.TempDir()

	setupAnswersDir(t, dir)
	path := writeAnswersFile(t, dir, "agent.yml", communityRepoURL)

	chdir(t, dir)

	require.NoError(t, RunE(nil, nil))

	assert.Equal(t, customBase+"/af-component-agent.git", readSrcPath(t, path))
}

func TestRunE_EnvVarSet_NoAnswersFiles(t *testing.T) {
	t.Setenv(copier.EnvApplicationTemplateGitBaseURL, "https://mirror.example.com")

	dir := t.TempDir()

	setupAnswersDir(t, dir)
	chdir(t, dir)

	require.NoError(t, RunE(nil, nil))
}
