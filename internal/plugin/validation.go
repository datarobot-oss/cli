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

package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/validate"
	"github.com/go-playground/validator/v10"
)

// validatePluginName checks that name is a safe single-segment identifier with no path separators.
// This prevents path traversal attacks when name is used to construct a filesystem path.
func validatePluginName(name string) error {
	if name == "" {
		return errors.New("plugin name must not be empty")
	}

	if strings.ContainsAny(name, `/\`) || name == ".." || name == "." {
		return fmt.Errorf("plugin name %q must not contain path separators or be a relative reference", name)
	}

	return nil
}

// ValidatePluginScript validates that a plugin script outputs a manifest matching the expected manifest.
// All fields must match exactly, including Scripts and MinCLIVersion for managed plugins.
func ValidatePluginScript(pluginDir string, expectedManifest PluginManifest) error {
	if err := ValidateLicense(pluginDir); err != nil {
		return err
	}

	scriptPath, err := FindPluginScript(pluginDir, expectedManifest.Name)
	if err != nil {
		return err
	}

	log.Debug("Validating plugin script outputs correct manifest", "script", scriptPath)

	scriptManifest, err := ExecPluginManifest(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to execute plugin script: %w", err)
	}

	return validateManifests(expectedManifest, *scriptManifest)
}

// ValidateLicense validates that a plugin has a LICENSE.txt file.
func ValidateLicense(pluginDir string) error {
	licensePath := filepath.Join(pluginDir, "LICENSE.txt")

	if _, err := os.Stat(licensePath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("plugin must contain LICENSE.txt file")
		}

		return fmt.Errorf("failed to check for LICENSE.txt: %w", err)
	}

	log.Debug("Plugin license validation passed", "path", licensePath)

	return nil
}

// createManifestValidatorOnce initializes the shared validator exactly once.
// validator.New() allocates reflection caches and RegisterValidation /
// RegisterTagNameFunc are not goroutine-safe to call concurrently, so
// sync.OnceValue guarantees a single initialization.
var createManifestValidatorOnce = sync.OnceValue(func() *validator.Validate {
	v := validator.New()

	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "" && name != "-" {
			return name
		}

		return f.Name
	})

	_ = validate.RegisterDRTags(v)

	return v
})

// validateManifests validates the script output manifest and checks that the
// core BasicPluginManifest fields match expected. Scripts and MinCLIVersion
// are intentionally ignored — they are optional managed-plugin fields that
// PATH plugins do not output.
//
// The field-by-field comparison below could be replaced with go-cmp, but was
// written out explicitly to avoid adding that dependency.
func validateManifests(expected, actual PluginManifest) error {
	manifestValidator := createManifestValidatorOnce()

	if err := manifestValidator.Struct(actual); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			return fmt.Errorf("plugin script output is invalid: field %q failed %q validation", ve[0].Field(), ve[0].Tag())
		}

		return fmt.Errorf("plugin script output is invalid: %w", err)
	}

	var mismatches []string

	// Fields `Scripts` and `MinCLIVersion` are ignored as they're optional managed plugin fields
	if actual.Name != expected.Name {
		mismatches = append(mismatches, fmt.Sprintf("Name: expected %q, got %q", expected.Name, actual.Name))
	}

	if actual.Version != expected.Version {
		mismatches = append(mismatches, fmt.Sprintf("Version: expected %q, got %q", expected.Version, actual.Version))
	}

	if actual.Description != expected.Description {
		mismatches = append(mismatches, fmt.Sprintf("Description: expected %q, got %q", expected.Description, actual.Description))
	}

	if actual.Authentication != expected.Authentication {
		mismatches = append(mismatches, fmt.Sprintf("Authentication: expected %v, got %v", expected.Authentication, actual.Authentication))
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("plugin script output does not match manifest.json:\n  %s", strings.Join(mismatches, "\n  "))
	}

	log.Debug("Plugin script manifest validation passed")

	return nil
}

// FindPluginScript finds the plugin script in the given directory.
func FindPluginScript(pluginDir, pluginName string) (string, error) {
	// First try exact match (no extension)
	scriptPath := filepath.Join(pluginDir, PluginPrefix+pluginName)

	if _, err := os.Stat(scriptPath); err == nil {
		return scriptPath, nil
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return "", fmt.Errorf("failed to read plugin directory: %w", err)
	}

	foundFiles := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		foundFiles = append(foundFiles, name)

		// Check for exact match or script with extensions
		if isPluginScript(name) {
			return filepath.Join(pluginDir, name), nil
		}
	}

	if len(foundFiles) == 0 {
		return "", fmt.Errorf("plugin script not found: expected '%s%s' in directory %s (directory is empty)", PluginPrefix, pluginName, pluginDir)
	}

	return "", fmt.Errorf("plugin script not found: expected '%s%s' in directory %s (found: %v)", PluginPrefix, pluginName, pluginDir, foundFiles)
}

func isPluginScript(name string) bool {
	prefixLen := len(PluginPrefix)
	if len(name) < prefixLen || name[:prefixLen] != PluginPrefix {
		return false
	}

	ext := filepath.Ext(name)

	// No extension is always valid (Unix executable)
	if ext == "" {
		return true
	}

	// Platform-specific extensions
	if runtime.GOOS == "windows" {
		return ext == ".ps1"
	}

	return ext == ".sh"
}

// ExecPluginManifest executes a plugin script and returns its manifest.
func ExecPluginManifest(scriptPath string) (*PluginManifest, error) {
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to make script executable: %w", err)
	}

	name, cmdArgs := pluginCommandArgs(scriptPath, PluginManifestFlag)

	cmd := exec.Command(name, cmdArgs...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("script error: %s", stderr.String())
		}

		return nil, fmt.Errorf("failed to execute: %w", err)
	}

	var manifest PluginManifest

	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON - check field names (use 'authenticated' not 'authentication'): %w\nOutput: %s", err, stdout.String())
	}

	return &manifest, nil
}
