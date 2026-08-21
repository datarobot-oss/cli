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

package copier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestApplyGitBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string // value to set in APPLICATION_TEMPLATE_GIT_BASE_URL (empty = unset)
		input   string
		want    string
	}{
		{
			name:    "no env var – community URL unchanged",
			baseURL: "",
			input:   "https://github.com/datarobot-community/af-component-agent.git",
			want:    "https://github.com/datarobot-community/af-component-agent.git",
		},
		{
			name:    "no env var – unrelated URL unchanged",
			baseURL: "",
			input:   "https://gitlab.com/acme/my-repo.git",
			want:    "https://gitlab.com/acme/my-repo.git",
		},
		{
			name:    "env var set – community URL replaced",
			baseURL: "https://internal.example.com/mirror",
			input:   "https://github.com/datarobot-community/af-component-agent.git",
			want:    "https://internal.example.com/mirror/af-component-agent.git",
		},
		{
			name:    "env var set – non-community datarobot URL unchanged",
			baseURL: "https://internal.example.com/mirror",
			input:   "https://github.com/datarobot/af-component-react.git",
			want:    "https://github.com/datarobot/af-component-react.git",
		},
		{
			name:    "env var set – unrelated URL unchanged",
			baseURL: "https://internal.example.com/mirror",
			input:   "https://gitlab.com/acme/my-repo.git",
			want:    "https://gitlab.com/acme/my-repo.git",
		},
		{
			name:    "env var set – trailing slash in env var is trimmed",
			baseURL: "https://internal.example.com/mirror/",
			input:   "https://github.com/datarobot-community/af-component-agent.git",
			want:    "https://internal.example.com/mirror/af-component-agent.git",
		},
		{
			name:    "env var set – community org base URL exactly (no trailing slash)",
			baseURL: "https://internal.example.com/mirror",
			input:   defaultGitBaseCommunity,
			want:    "https://internal.example.com/mirror",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvApplicationTemplateGitBaseURL, tt.baseURL)

			got := ApplyGitBaseURL(tt.input)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRewriteSrcPathIfNeeded(t *testing.T) {
	const customBase = "https://internal.example.com/mirror"

	communityRepoURL := "https://github.com/datarobot-community/af-component-agent.git"
	mainOrgRepoURL := "https://github.com/datarobot/af-component-react.git"
	otherRepoURL := "https://gitlab.com/acme/my-repo.git"

	makeAnswersFile := func(t *testing.T, dir, srcPath string) string {
		t.Helper()

		content := "_src_path: " + srcPath + "\nsome_answer: value\n"
		path := filepath.Join(dir, "component.yml")

		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		return path
	}

	readSrcPath := func(t *testing.T, path string) string {
		t.Helper()

		data, err := os.ReadFile(path)
		require.NoError(t, err)

		var result struct {
			SrcPath string `yaml:"_src_path"`
		}

		require.NoError(t, yaml.Unmarshal(data, &result))

		return result.SrcPath
	}

	t.Run("no env var – file unchanged", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, "")
		dir := t.TempDir()
		path := makeAnswersFile(t, dir, communityRepoURL)
		original, _ := os.ReadFile(path)

		require.NoError(t, RewriteSrcPathIfNeeded(path))

		after, _ := os.ReadFile(path)
		assert.Equal(t, string(original), string(after))
	})

	t.Run("env var set – community URL rewritten", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, customBase)
		dir := t.TempDir()
		path := makeAnswersFile(t, dir, communityRepoURL)

		require.NoError(t, RewriteSrcPathIfNeeded(path))

		assert.Equal(t, customBase+"/af-component-agent.git", readSrcPath(t, path))
	})

	t.Run("env var set – non-community datarobot URL not rewritten", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, customBase)
		dir := t.TempDir()
		path := makeAnswersFile(t, dir, mainOrgRepoURL)

		require.NoError(t, RewriteSrcPathIfNeeded(path))

		assert.Equal(t, mainOrgRepoURL, readSrcPath(t, path))
	})

	t.Run("env var set – unrelated URL unchanged", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, customBase)
		dir := t.TempDir()
		path := makeAnswersFile(t, dir, otherRepoURL)

		require.NoError(t, RewriteSrcPathIfNeeded(path))

		assert.Equal(t, otherRepoURL, readSrcPath(t, path))
	})

	t.Run("env var set – already uses custom URL, file unchanged", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, customBase)
		dir := t.TempDir()
		alreadyRewritten := customBase + "/af-component-agent.git"
		path := makeAnswersFile(t, dir, alreadyRewritten)
		original, _ := os.ReadFile(path)

		require.NoError(t, RewriteSrcPathIfNeeded(path))

		after, _ := os.ReadFile(path)
		assert.Equal(t, string(original), string(after))
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		t.Setenv(EnvApplicationTemplateGitBaseURL, customBase)

		err := RewriteSrcPathIfNeeded("/nonexistent/path/component.yml")
		assert.Error(t, err)
	})
}
