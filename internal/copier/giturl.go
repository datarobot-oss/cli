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
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// EnvApplicationTemplateGitBaseURL is the environment variable name used to
	// override the default DataRobot community GitHub base URL for component templates.
	EnvApplicationTemplateGitBaseURL = "APPLICATION_TEMPLATE_GIT_BASE_URL"

	// defaultGitBaseCommunity is the DataRobot community GitHub org base URL.
	defaultGitBaseCommunity = "https://github.com/datarobot-community"
)

// ApplyGitBaseURL replaces the DataRobot community GitHub base URL
// (https://github.com/datarobot-community) with the value of
// APPLICATION_TEMPLATE_GIT_BASE_URL when that variable is set.
// If the variable is not set, or the URL does not start with the community prefix,
// the original URL is returned unchanged.
func ApplyGitBaseURL(url string) string {
	baseURL := os.Getenv(EnvApplicationTemplateGitBaseURL)
	if baseURL == "" {
		return url
	}

	baseURL = strings.TrimRight(baseURL, "/")

	if strings.HasPrefix(url, defaultGitBaseCommunity+"/") || url == defaultGitBaseCommunity {
		return baseURL + url[len(defaultGitBaseCommunity):]
	}

	return url
}

// RewriteSrcPathIfNeeded rewrites the _src_path field in a copier answers file
// if APPLICATION_TEMPLATE_GIT_BASE_URL is set and the current _src_path contains
// one of the default DataRobot GitHub base URLs.
//
// This is necessary so that `dr component update` clones from the custom mirror
// rather than from GitHub, even when the component was originally added before
// the environment variable was configured.
//
// The function is a no-op when APPLICATION_TEMPLATE_GIT_BASE_URL is unset or
// when the _src_path already uses the desired base URL.
func RewriteSrcPathIfNeeded(yamlFile string) error {
	baseURL := os.Getenv(EnvApplicationTemplateGitBaseURL)
	if baseURL == "" {
		return nil
	}

	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return fmt.Errorf("failed to read answers file %q: %w", yamlFile, err)
	}

	// Parse the entire file into a generic map so we can update just _src_path
	// while leaving all other fields intact.
	var answersMap map[string]interface{}

	if err := yaml.Unmarshal(data, &answersMap); err != nil {
		return fmt.Errorf("failed to parse answers file %q: %w", yamlFile, err)
	}

	srcPath, _ := answersMap["_src_path"].(string)
	if srcPath == "" {
		return nil
	}

	newSrcPath := ApplyGitBaseURL(srcPath)
	if newSrcPath == srcPath {
		return nil
	}

	answersMap["_src_path"] = newSrcPath

	updated, err := yaml.Marshal(answersMap)
	if err != nil {
		return fmt.Errorf("failed to marshal updated answers file %q: %w", yamlFile, err)
	}

	if err := os.WriteFile(yamlFile, updated, 0o644); err != nil {
		return fmt.Errorf("failed to write updated answers file %q: %w", yamlFile, err)
	}

	return nil
}
