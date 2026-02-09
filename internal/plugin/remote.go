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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/codeclysm/extract/v4"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	"github.com/ulikunitz/xz"
)

const (
	registryFetchTimeout  = 30 * time.Second
	pluginDownloadTimeout = 5 * time.Minute  // Future note: this might need to be configurable for very large plugins
	httpDialTimeout       = 30 * time.Second // Connection timeout - fail fast if no internet
)

// FetchRegistry downloads and parses the plugin registry from the remote URL.
// Returns the registry, the base URL (directory of registry file), and any error.
func FetchRegistry(registryURL string) (*PluginRegistry, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registryFetchTimeout)
	defer cancel()

	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", config.GetUserAgentHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Failed to fetch registry: HTTP %d", resp.StatusCode)
	}

	var registry PluginRegistry

	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return nil, "", fmt.Errorf("Failed to parse registry: %w", err)
	}

	// Extract base URL (directory containing index.json)
	baseURL := registryURL
	if idx := strings.LastIndex(baseURL, "/"); idx > 0 {
		baseURL = baseURL[:idx]
	}

	return &registry, baseURL, nil
}

// ResolveVersion finds the best matching version for a constraint
// Supports full semver constraint syntax including: exact (1.2.3), caret (^1.2.3), tilde (~1.2.3),
// ranges (>=1.0.0), comma-separated constraints (>=1.0.0, <2.0.0), and latest
func ResolveVersion(versions []RegistryVersion, constraint string) (*RegistryVersion, error) {
	if len(versions) == 0 {
		return nil, errors.New("No versions available")
	}

	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "latest" {
		return findLatestVersion(versions)
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, fmt.Errorf("Invalid version constraint '%s': %w", constraint, err)
	}

	return findBestMatch(versions, c)
}

func findLatestVersion(versions []RegistryVersion) (*RegistryVersion, error) {
	if len(versions) == 0 {
		return nil, errors.New("No versions available")
	}

	var latest *RegistryVersion

	var latestVer *semver.Version

	for i := range versions {
		v, err := semver.NewVersion(versions[i].Version)
		if err != nil {
			continue
		}

		if latestVer == nil || v.GreaterThan(latestVer) {
			latestVer = v
			latest = &versions[i]
		}
	}

	if latest == nil {
		return nil, errors.New("No valid semver versions found")
	}

	return latest, nil
}

func findBestMatch(versions []RegistryVersion, constraint *semver.Constraints) (*RegistryVersion, error) {
	var candidates []*semver.Version

	var candidateIndexes []int

	for i := range versions {
		v, err := semver.NewVersion(versions[i].Version)
		if err != nil {
			continue
		}

		if constraint.Check(v) {
			candidates = append(candidates, v)
			candidateIndexes = append(candidateIndexes, i)
		}
	}

	if len(candidates) == 0 {
		return nil, errors.New("No version matching constraint found")
	}

	bestIdx := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].GreaterThan(candidates[bestIdx]) {
			bestIdx = i
		}
	}

	return &versions[candidateIndexes[bestIdx]], nil
}

// InstallPlugin downloads and installs a plugin
func InstallPlugin(pluginEntry RegistryPlugin, version RegistryVersion, baseURL string) error {
	pluginDir, err := preparePluginDirectory(pluginEntry.Name)
	if err != nil {
		return err
	}

	archivePath, err := downloadAndVerifyPlugin(version, baseURL)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	return installPluginFromArchive(archivePath, pluginDir, pluginEntry, version)
}

func preparePluginDirectory(name string) (string, error) {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return "", fmt.Errorf("Failed to get plugins directory: %w", err)
	}

	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return "", fmt.Errorf("Failed to create plugins directory: %w", err)
	}

	return filepath.Join(managedDir, name), nil
}

func downloadAndVerifyPlugin(version RegistryVersion, baseURL string) (string, error) {
	log.Debug("Downloading plugin", "url", version.URL)

	archivePath, err := downloadFile(version.URL, baseURL)
	if err != nil {
		return "", fmt.Errorf("Failed to download plugin: %w", err)
	}

	// Require SHA256 for remote downloads (not for file:// URLs used in testing)
	isRemote := strings.HasPrefix(version.URL, "http://") || strings.HasPrefix(version.URL, "https://")
	if isRemote && version.SHA256 == "" {
		os.Remove(archivePath)

		return "", errors.New("Plugin version missing required SHA256 checksum")
	}

	if version.SHA256 != "" {
		if err := verifyChecksum(archivePath, version.SHA256); err != nil {
			os.Remove(archivePath)

			return "", fmt.Errorf("Checksum verification failed: %w", err)
		}
	}

	return archivePath, nil
}

func installPluginFromArchive(archivePath, pluginDir string, entry RegistryPlugin, version RegistryVersion) error {
	if _, err := os.Stat(pluginDir); err == nil {
		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("Failed to remove existing installation: %w", err)
		}
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("Failed to create plugin directory: %w", err)
	}

	if err := extractTarXz(archivePath, pluginDir); err != nil {
		os.RemoveAll(pluginDir)

		return fmt.Errorf("Failed to extract plugin: %w", err)
	}

	if err := saveInstalledMetadata(pluginDir, entry, version); err != nil {
		log.Warn("Failed to save installation metadata", "error", err)
	}

	return nil
}

// UninstallPlugin removes an installed plugin
func UninstallPlugin(name string) error {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return fmt.Errorf("Failed to get plugins directory: %w", err)
	}

	pluginDir := filepath.Join(managedDir, name)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("Plugin %s is not installed", name)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("Failed to remove plugin: %w", err)
	}

	return nil
}

// GetInstalledPlugins returns metadata about installed managed plugins
func GetInstalledPlugins() ([]InstalledPlugin, error) {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(managedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	installed := make([]InstalledPlugin, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		meta := loadPluginMetadata(managedDir, entry.Name())
		installed = append(installed, meta)
	}

	return installed, nil
}

func loadPluginMetadata(managedDir, name string) InstalledPlugin {
	metadataPath := filepath.Join(managedDir, name, ".installed.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return InstalledPlugin{
			Name:    name,
			Version: "unknown",
		}
	}

	var meta InstalledPlugin

	if err := json.Unmarshal(data, &meta); err != nil {
		return InstalledPlugin{
			Name:    name,
			Version: "unknown",
		}
	}

	return meta
}

// downloadFile downloads a file to a temporary location and returns the path.
// If the URL is relative, it is resolved against baseURL.
func downloadFile(url, baseURL string) (string, error) {
	// Handle file:// URLs for testing
	if strings.HasPrefix(url, "file://") {
		return copyFileURL(url)
	}

	// Resolve relative URLs
	finalURL := url
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		// Relative URL - join with base
		finalURL = baseURL + "/" + strings.TrimPrefix(url, "/")
	}

	return downloadHTTP(finalURL)
}

// copyFileURL handles file:// URLs by copying to a temp file
func copyFileURL(url string) (string, error) {
	srcPath := strings.TrimPrefix(url, "file://")

	tmpFile, err := os.CreateTemp("", "dr-plugin-*.tar.xz")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	srcFile, err := os.Open(srcPath)
	if err != nil {
		os.Remove(tmpFile.Name())

		return "", err
	}
	defer srcFile.Close()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		os.Remove(tmpFile.Name())

		return "", err
	}

	return tmpFile.Name(), nil
}

// downloadHTTP downloads a file via HTTP to a temp file
func downloadHTTP(finalURL string) (string, error) {
	log.Debug("Downloading plugin", "url", finalURL)

	ctx, cancel := context.WithTimeout(context.Background(), pluginDownloadTimeout)
	defer cancel()

	// Custom transport with connection timeout to fail fast if no internet
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: httpDialTimeout,
		}).DialContext,
	}

	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, finalURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "dr-plugin-*.tar.xz")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())

		return "", err
	}

	return tmpFile.Name(), nil
}

// verifyChecksum verifies the SHA256 checksum of a file
func verifyChecksum(filePath, expected string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("Expected %s, got %s", expected, actual)
	}

	return nil
}

// extractTarXz extracts a .tar.xz archive to the destination directory
// Uses secure extraction library to prevent path traversal, zip bombs, and symlink attacks
func extractTarXz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	xzReader, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("Failed to create xz reader: %w", err)
	}

	// Use extract library for secure extraction with built-in protections
	return extract.Tar(context.Background(), xzReader, destDir, nil)
}

// saveInstalledMetadata saves metadata about the installed plugin
func saveInstalledMetadata(pluginDir string, entry RegistryPlugin, version RegistryVersion) error {
	meta := InstalledPlugin{
		Name:        entry.Name,
		Version:     version.Version,
		Source:      version.URL,
		InstalledAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(pluginDir, ".installed.json"), data, 0o644)
}

// BackupPlugin creates a backup of an installed plugin in a temporary directory
// Returns the path to the backup directory
func BackupPlugin(name string) (string, error) {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return "", fmt.Errorf("Failed to get plugins directory: %w", err)
	}

	pluginDir := filepath.Join(managedDir, name)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return "", fmt.Errorf("Plugin %s is not installed", name)
	}

	backupDir, err := os.MkdirTemp("", fmt.Sprintf("dr-plugin-backup-%s-*", name))
	if err != nil {
		return "", fmt.Errorf("Failed to create backup directory: %w", err)
	}

	if err := copyDir(pluginDir, backupDir); err != nil {
		os.RemoveAll(backupDir)

		return "", fmt.Errorf("Failed to copy plugin to backup: %w", err)
	}

	return backupDir, nil
}

// RestorePlugin restores a plugin from a backup directory
func RestorePlugin(name, backupPath string) error {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return fmt.Errorf("Failed to get plugins directory: %w", err)
	}

	pluginDir := filepath.Join(managedDir, name)

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("Failed to remove corrupted plugin: %w", err)
	}

	if err := copyDir(backupPath, pluginDir); err != nil {
		return fmt.Errorf("Failed to restore plugin from backup: %w", err)
	}

	return nil
}

// CleanupBackup removes a backup directory
func CleanupBackup(backupPath string) {
	if backupPath != "" {
		os.RemoveAll(backupPath)
	}
}

// ValidatePlugin validates that a plugin installation is working correctly
func ValidatePlugin(name string) error {
	managedDir, err := ManagedPluginsDir()
	if err != nil {
		return fmt.Errorf("failed to get plugins directory: %w", err)
	}

	pluginDir := filepath.Join(managedDir, name)

	metadataPath := filepath.Join(pluginDir, ".installed.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return errors.New("Plugin metadata not found")
	}

	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return errors.New("Plugin manifest not found")
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("Failed to read manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("Failed to parse manifest: %w", err)
	}

	if manifest.Name != name {
		return fmt.Errorf("Manifest name mismatch: expected %s, got %s", name, manifest.Name)
	}

	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)

	return err
}
