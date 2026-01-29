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
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/repo"
	"github.com/ulikunitz/xz"
)

// FetchIndex downloads and parses the plugin index from the remote URL
func FetchIndex(indexURL string) (*PluginIndex, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", config.GetUserAgentHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch index: HTTP %d", resp.StatusCode)
	}

	var index PluginIndex

	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	return &index, nil
}

// ResolveVersion finds the best matching version for a constraint
// Supports: exact (1.2.3), caret (^1.2.3), tilde (~1.2.3), range (>=1.0.0), latest
func ResolveVersion(versions []IndexVersion, constraint string) (*IndexVersion, error) {
	if len(versions) == 0 {
		return nil, errors.New("no versions available")
	}

	// Sort versions descending (newest first)
	sorted := make([]IndexVersion, len(versions))
	copy(sorted, versions)

	sort.Slice(sorted, func(i, j int) bool {
		return compareVersions(sorted[i].Version, sorted[j].Version) > 0
	})

	return resolveConstraint(sorted, constraint)
}

func resolveConstraint(sorted []IndexVersion, constraint string) (*IndexVersion, error) {
	constraint = strings.TrimSpace(constraint)

	if constraint == "" || constraint == "latest" {
		return &sorted[0], nil
	}

	if !strings.ContainsAny(constraint, "^~<>=") {
		return findExactVersion(sorted, constraint)
	}

	if strings.HasPrefix(constraint, "^") {
		return resolveCaretConstraint(sorted, constraint)
	}

	if strings.HasPrefix(constraint, "~") {
		return resolveTildeConstraint(sorted, constraint)
	}

	if strings.HasPrefix(constraint, ">=") {
		return resolveGTEConstraint(sorted, constraint)
	}

	return nil, fmt.Errorf("unsupported version constraint: %s", constraint)
}

func findExactVersion(sorted []IndexVersion, constraint string) (*IndexVersion, error) {
	for i := range sorted {
		if sorted[i].Version == constraint || sorted[i].Version == "v"+constraint {
			return &sorted[i], nil
		}
	}

	return nil, fmt.Errorf("version %s not found", constraint)
}

func resolveCaretConstraint(sorted []IndexVersion, constraint string) (*IndexVersion, error) {
	target := strings.TrimPrefix(constraint, "^")
	majorTarget := parseMajor(target)

	for i := range sorted {
		major := parseMajor(sorted[i].Version)
		if major == majorTarget && compareVersions(sorted[i].Version, target) >= 0 {
			return &sorted[i], nil
		}
	}

	return nil, fmt.Errorf("no version matching %s found", constraint)
}

func resolveTildeConstraint(sorted []IndexVersion, constraint string) (*IndexVersion, error) {
	target := strings.TrimPrefix(constraint, "~")
	majorTarget, minorTarget := parseMajorMinor(target)

	for i := range sorted {
		major, minor := parseMajorMinor(sorted[i].Version)
		if major == majorTarget && minor == minorTarget && compareVersions(sorted[i].Version, target) >= 0 {
			return &sorted[i], nil
		}
	}

	return nil, fmt.Errorf("no version matching %s found", constraint)
}

func resolveGTEConstraint(sorted []IndexVersion, constraint string) (*IndexVersion, error) {
	target := strings.TrimPrefix(constraint, ">=")

	for i := range sorted {
		if compareVersions(sorted[i].Version, target) >= 0 {
			return &sorted[i], nil
		}
	}

	return nil, fmt.Errorf("no version matching %s found", constraint)
}

// InstallPlugin downloads and installs a plugin
func InstallPlugin(pluginEntry IndexPlugin, version IndexVersion) error {
	pluginDir, err := preparePluginDirectory(pluginEntry.Name)
	if err != nil {
		return err
	}

	archivePath, err := downloadAndVerifyPlugin(version)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	return installPluginFromArchive(archivePath, pluginDir, pluginEntry, version)
}

func preparePluginDirectory(name string) (string, error) {
	managedDir, err := repo.ManagedPluginsDir()
	if err != nil {
		return "", fmt.Errorf("failed to get plugins directory: %w", err)
	}

	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create plugins directory: %w", err)
	}

	return filepath.Join(managedDir, name), nil
}

func downloadAndVerifyPlugin(version IndexVersion) (string, error) {
	log.Debug("Downloading plugin", "url", version.URL)

	archivePath, err := downloadFile(version.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download plugin: %w", err)
	}

	if version.SHA256 != "" {
		if err := verifyChecksum(archivePath, version.SHA256); err != nil {
			os.Remove(archivePath)

			return "", fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	return archivePath, nil
}

func installPluginFromArchive(archivePath, pluginDir string, entry IndexPlugin, version IndexVersion) error {
	if _, err := os.Stat(pluginDir); err == nil {
		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("failed to remove existing installation: %w", err)
		}
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if err := extractTarXz(archivePath, pluginDir); err != nil {
		os.RemoveAll(pluginDir)

		return fmt.Errorf("failed to extract plugin: %w", err)
	}

	if err := makeScriptsExecutable(pluginDir); err != nil {
		log.Warn("Failed to make scripts executable", "error", err)
	}

	if err := saveInstalledMetadata(pluginDir, entry, version); err != nil {
		log.Warn("Failed to save installation metadata", "error", err)
	}

	return nil
}

// UninstallPlugin removes an installed plugin
func UninstallPlugin(name string) error {
	managedDir, err := repo.ManagedPluginsDir()
	if err != nil {
		return fmt.Errorf("failed to get plugins directory: %w", err)
	}

	pluginDir := filepath.Join(managedDir, name)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %s is not installed", name)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin: %w", err)
	}

	return nil
}

// GetInstalledPlugins returns metadata about installed managed plugins
func GetInstalledPlugins() ([]InstalledPlugin, error) {
	managedDir, err := repo.ManagedPluginsDir()
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

// downloadFile downloads a file to a temporary location and returns the path
func downloadFile(url string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, err := http.NewRequest(http.MethodGet, url, nil)
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
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}

	return nil
}

// extractTarXz extracts a .tar.xz archive to the destination directory
func extractTarXz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	xzReader, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create xz reader: %w", err)
	}

	tarReader := tar.NewReader(xzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if err := extractTarEntry(tarReader, header, destDir); err != nil {
			return err
		}
	}

	return nil
}

func extractTarEntry(tarReader *tar.Reader, header *tar.Header, destDir string) error {
	cleanName := filepath.Clean(header.Name)

	// Skip root directory entries (. or ./)
	if cleanName == "." || cleanName == "" {
		return nil
	}

	targetPath := filepath.Join(destDir, cleanName)

	if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path in archive: %s", header.Name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(targetPath, 0o755)
	case tar.TypeReg:
		return extractRegularFile(tarReader, targetPath, header.Mode)
	}

	return nil
}

func extractRegularFile(tarReader *tar.Reader, targetPath string, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, tarReader)

	return err
}

// makeScriptsExecutable sets executable permissions on script files
func makeScriptsExecutable(pluginDir string) error {
	return filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext == ".sh" || ext == "" {
			return os.Chmod(path, 0o755)
		}

		return nil
	})
}

// saveInstalledMetadata saves metadata about the installed plugin
func saveInstalledMetadata(pluginDir string, entry IndexPlugin, version IndexVersion) error {
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

// Version comparison helpers

var semverRegex = regexp.MustCompile(`v?(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

func parseVersion(v string) (major, minor, patch int) {
	matches := semverRegex.FindStringSubmatch(v)
	if len(matches) < 2 {
		return 0, 0, 0
	}

	major, _ = strconv.Atoi(matches[1])

	if len(matches) >= 3 && matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}

	if len(matches) >= 4 && matches[3] != "" {
		patch, _ = strconv.Atoi(matches[3])
	}

	return major, minor, patch
}

func parseMajor(v string) int {
	major, _, _ := parseVersion(v)

	return major
}

func parseMajorMinor(v string) (int, int) {
	major, minor, _ := parseVersion(v)

	return major, minor
}

func compareVersions(a, b string) int {
	aMaj, aMin, aPatch := parseVersion(a)
	bMaj, bMin, bPatch := parseVersion(b)

	if aMaj != bMaj {
		return aMaj - bMaj
	}

	if aMin != bMin {
		return aMin - bMin
	}

	return aPatch - bPatch
}
