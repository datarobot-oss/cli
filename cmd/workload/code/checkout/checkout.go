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

package checkout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
)

const (
	checkoutMetaFile = ".checkout-meta.json"

	// Standard Unix modes; the snapshot is for the invoking user's own
	// inspection, not multi-user sharing — modes here aren't load-bearing
	// for safety, only for letting the user read/traverse what we wrote.
	checkoutDirPerm      = 0o755
	checkoutMetaFilePerm = 0o644
)

type checkoutMeta struct {
	VersionID    string    `json:"versionId"`
	CheckedOutAt time.Time `json:"checkedOutAt"`
	FileCount    int       `json:"fileCount"`
	TotalSize    int64     `json:"totalSize"`
}

func runDownload(out io.Writer, format workload.OutputFormat, dir, verArg string, deps Deps) error {
	startedAt := time.Now()

	pre, err := preflight(dir, verArg, deps)
	if err != nil {
		return err
	}

	files, err := deps.Files.AllFiles(pre.catalogID, pre.versionID)
	if err != nil {
		return fmt.Errorf("list files for version %s: %w", pre.versionID, err)
	}

	var totalSize int64

	for _, m := range files {
		totalSize += m.Size
	}

	parent := wapi.CheckoutsDir(dir)

	if err := prepareCheckoutsParent(parent, totalSize); err != nil {
		return err
	}

	if err := tui.RunWithSpinner(
		fmt.Sprintf("Downloading %d file(s)…", len(files)),
		func() error { return stageAndInstall(deps.Files, pre, parent, files, totalSize, startedAt) },
	); err != nil {
		return err
	}

	if err := wapi.AppendHistory(dir, checkoutHistoryEntry(startedAt, pre.versionID, len(files), totalSize, time.Since(startedAt))); err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	return renderDownloadResult(out, format, downloadView(dir, pre.versionID, pre.checkoutDir, files))
}

// stageAndInstall downloads files into a sibling temp dir, writes metadata,
// and atomically swaps it into place. The temp dir is removed on any failure
// before the swap; after a successful swap it has been renamed away.
func stageAndInstall(c filesapi.Client, pre preflightResult, parent string, files map[string]filesapi.FileMeta, totalSize int64, startedAt time.Time) error {
	// Dot-prefix so listCheckoutNames skips it; sibling of finalDir so the swap rename is intra-filesystem.
	tempDir, err := os.MkdirTemp(parent, ".tmp-"+pre.versionID+"-")
	if err != nil {
		return fmt.Errorf("create temp checkout dir: %w", err)
	}

	installed := false

	defer func() {
		if !installed {
			_ = os.RemoveAll(tempDir)
		}
	}()

	if err := downloadAll(c, pre.catalogID, pre.versionID, tempDir, files); err != nil {
		return err
	}

	meta := checkoutMeta{
		VersionID:    pre.versionID,
		CheckedOutAt: startedAt.UTC(),
		FileCount:    len(files),
		TotalSize:    totalSize,
	}

	if err := writeCheckoutMeta(tempDir, meta); err != nil {
		return err
	}

	if err := swapCheckoutDir(tempDir, pre.checkoutDir); err != nil {
		return err
	}

	installed = true

	return nil
}

// Backup-rename swap so a failed install can be rolled back to the previous snapshot.
func swapCheckoutDir(tempDir, finalDir string) error {
	backupDir := filepath.Join(filepath.Dir(finalDir), ".bak-"+filepath.Base(finalDir))

	// Clear any leftover backup from a previously interrupted swap.
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("clear stale backup dir: %w", err)
	}

	var hasOld bool

	// TOCTOU window: another process could create or remove finalDir between
	// the Stat and the Rename. Acceptable for a single-user CLI — concurrent
	// `dr` invocations against the same checkout dir are not supported.
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, backupDir); err != nil {
			return fmt.Errorf("back up existing checkout dir: %w", err)
		}

		hasOld = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat existing checkout dir: %w", err)
	}

	if err := os.Rename(tempDir, finalDir); err != nil {
		installErr := fmt.Errorf("install checkout dir: %w", err)

		if hasOld {
			if rbErr := os.Rename(backupDir, finalDir); rbErr != nil {
				return errors.Join(
					installErr,
					fmt.Errorf("rollback failed; previous snapshot stranded at %s: %w", backupDir, rbErr),
				)
			}
		}

		return installErr
	}

	if hasOld {
		_ = os.RemoveAll(backupDir)
	}

	return nil
}

func checkoutHistoryEntry(startedAt time.Time, versionID string, fileCount int, totalSize int64, duration time.Duration) wapi.HistoryEntry {
	return wapi.HistoryEntry{
		"ts":       startedAt.UTC().Format(time.RFC3339),
		"op":       "checkout",
		"version":  versionID,
		"files":    fileCount,
		"size":     totalSize,
		"duration": duration.Round(time.Millisecond).String(),
	}
}

type preflightResult struct {
	catalogID   string
	versionID   string
	checkoutDir string
}

func preflight(dir, verArg string, deps Deps) (preflightResult, error) {
	cfg, err := wapi.LoadConfig(dir)
	if err != nil {
		return preflightResult{}, fmt.Errorf("read .wapi/config.json: %w", err)
	}

	if cfg.CatalogID == nil || *cfg.CatalogID == "" {
		return preflightResult{}, errors.New("no code has been synced yet. Run 'dr workload code sync' first")
	}

	if err := probeArtifact(deps.GetArtifact, cfg.ArtifactID); err != nil {
		return preflightResult{}, err
	}

	versionID, err := resolveVersion(deps.Files, *cfg.CatalogID, verArg)
	if err != nil {
		return preflightResult{}, err
	}

	return preflightResult{
		catalogID:   *cfg.CatalogID,
		versionID:   versionID,
		checkoutDir: wapi.CheckoutDir(dir, versionID),
	}, nil
}

func prepareCheckoutsParent(parent string, totalSize int64) error {
	if err := os.MkdirAll(parent, checkoutDirPerm); err != nil {
		return fmt.Errorf("create .wapi/.checkouts dir: %w", err)
	}

	if err := sync.EnsureSpaceFor(parent, totalSize); err != nil {
		return err
	}

	return nil
}

// probeArtifact surfaces a 404 before any download work begins. The artifact
// payload is discarded; preflight only needs to confirm the ID resolves.
func probeArtifact(get func(string) (*workload.Artifact, error), artifactID string) error {
	if _, err := get(artifactID); err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("artifact %s not found", artifactID)
		}

		return fmt.Errorf("fetch artifact %s: %w", artifactID, err)
	}

	return nil
}

// Accepts the full ID or any unique prefix.
func resolveVersion(c filesapi.Client, catalogID, arg string) (string, error) {
	if arg == "" {
		return "", errors.New("version argument is empty")
	}

	versions, err := c.ListVersions(catalogID, 0)
	if err != nil {
		return "", fmt.Errorf("list versions: %w", err)
	}

	var matches []string

	for _, v := range versions {
		if v.ID == arg {
			return v.ID, nil
		}

		if strings.HasPrefix(v.ID, arg) {
			matches = append(matches, v.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("version %q not found", arg)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("version prefix %q is ambiguous — matches %d versions; use more characters", arg, len(matches))
	}
}

func downloadAll(c filesapi.Client, catalogID, versionID, checkoutDir string, files map[string]filesapi.FileMeta) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	for _, path := range paths {
		if err := fileops.SafeRelPath(path); err != nil {
			return fmt.Errorf("server returned unsafe path %q: %w", path, err)
		}
	}

	for _, path := range paths {
		if err := downloadOne(c, catalogID, versionID, checkoutDir, path); err != nil {
			return err
		}
	}

	return nil
}

// Server-side hashes are not re-verified: TLS covers transit corruption and the snapshot is read-only.
func downloadOne(c filesapi.Client, catalogID, versionID, checkoutDir, path string) error {
	dst := filepath.Join(checkoutDir, filepath.FromSlash(path))

	if err := os.MkdirAll(filepath.Dir(dst), checkoutDirPerm); err != nil {
		return fmt.Errorf("mkdir parent for %s: %w", path, err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	_, _, err = c.DownloadFile(catalogID, versionID, path, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		_ = os.Remove(dst)

		return fmt.Errorf("download %s: %w", path, err)
	}

	return nil
}

func writeCheckoutMeta(checkoutDir string, meta checkoutMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode checkout meta: %w", err)
	}

	path := filepath.Join(checkoutDir, checkoutMetaFile)
	if err := os.WriteFile(path, data, checkoutMetaFilePerm); err != nil {
		return fmt.Errorf("write checkout meta: %w", err)
	}

	return nil
}

func runClean(out io.Writer, format workload.OutputFormat, dir, arg string) error {
	checkoutsDir := wapi.CheckoutsDir(dir)

	names, err := listCheckoutNames(checkoutsDir)
	if err != nil {
		return err
	}

	if arg == "" {
		if len(names) == 0 {
			return renderCleanResult(out, format, cleanResult{Removed: []string{}})
		}

		if err := os.RemoveAll(checkoutsDir); err != nil {
			return fmt.Errorf("remove %s: %w", checkoutsDir, err)
		}

		return renderCleanResult(out, format, cleanResult{Removed: names})
	}

	resolved, err := resolveLocalCheckout(names, arg)
	if err != nil {
		return err
	}

	target := wapi.CheckoutDir(dir, resolved)
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove %s: %w", target, err)
	}

	return renderCleanResult(out, format, cleanResult{Removed: []string{resolved}})
}

// Exact name wins; otherwise arg is treated as a unique prefix.
func resolveLocalCheckout(names []string, arg string) (string, error) {
	if arg == "" {
		return "", errors.New("checkout argument is empty")
	}

	var matches []string

	for _, n := range names {
		if n == arg {
			return n, nil
		}

		if strings.HasPrefix(n, arg) {
			matches = append(matches, n)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("checkout %q not found locally", arg)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("checkout prefix %q is ambiguous — matches %d directories", arg, len(matches))
	}
}

// Missing parent dir yields an empty slice with no error.
func listCheckoutNames(checkoutsDir string) ([]string, error) {
	entries, err := os.ReadDir(checkoutsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read %s: %w", checkoutsDir, err)
	}

	names := make([]string, 0, len(entries))

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	return names, nil
}
