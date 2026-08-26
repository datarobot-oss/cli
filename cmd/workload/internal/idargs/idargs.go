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

// Package idargs centralizes the optional <workload-id> positional shared by
// the dr workload leaves: each accepts the id or omits it, in which case the
// workloadId bound in the nearest .datarobot.yaml is used. Keeping the arity,
// the --dir flag and the help text in one place stops seven commands drifting
// apart.
//
// It sits here rather than beside ResolveArtifactID in internal/workload
// because that package cannot import internal/workload/manifest: the manifest
// package's own tests import it back, and the cycle is only visible when the
// tests build.
package idargs

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// HelpText is the paragraph every leaf appends to its Long, so one behaviour
// is not explained seven ways.
const HelpText = `The workload id is optional in a project directory: without it, the
workloadId bound in the nearest .datarobot.yaml is used, searched upward
from --dir (the current directory by default).`

// AddDirFlag registers --dir. Paired with Resolve so a command cannot read the
// flag without offering it.
func AddDirFlag(cmd *cobra.Command) {
	cmd.Flags().String("dir", "",
		"Project directory whose .datarobot.yaml names the workload, searched upward from here.")
}

// Resolve returns the workload the command should act on, announcing one read
// from a manifest on stderr.
//
// The notice is printed here rather than at each call site so seven commands
// cannot each decide differently whether to say.
func Resolve(cmd *cobra.Command, args []string) (Ref, error) {
	dir, err := dirFlag(cmd)
	if err != nil {
		return Ref{}, err
	}

	ref, err := resolve(args, dir)
	if err != nil {
		return Ref{}, err
	}

	announce(cmd, ref)

	return ref, nil
}

// announce names a workload the user did not, so an ambient target is never
// silent. JSON runs stay quiet: stdout purity is only half the contract, and
// `--output-format json 2>&1 | jq .` has to parse too.
func announce(cmd *cobra.Command, ref Ref) {
	if !ref.FromManifest() || outputformat.GetFormat(cmd) == outputformat.OutputFormatJSON {
		return
	}

	fmt.Fprintln(cmd.ErrOrStderr(),
		tui.DimStyle.Render("Using workload "+ref.ID+" from "+ref.Path))
}

// dirFlag reads --dir and defaults it to where the shell is standing.
//
// The lookup error is not discarded: it means the flag was renamed or dropped,
// and the silent fallback is a search from the working directory, which is the
// behaviour --dir exists to replace.
func dirFlag(cmd *cobra.Command) (string, error) {
	dir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return "", fmt.Errorf("cannot read --dir: %w", err)
	}

	if dir != "" {
		if !fsutil.DirExists(dir) {
			return "", fmt.Errorf("--dir %s: %w", dir, manifest.ErrNotADirectory)
		}

		return dir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine the current directory: %w", err)
	}

	return cwd, nil
}

const (
	WorkloadIDSourceExplicit = "explicit"
	WorkloadIDSourceManifest = "manifest"
)

// Ref is the workload a command was pointed at, and how it was named.
type Ref struct {
	ID     string
	Source string
	// Path is the manifest the id came from, "" when the id was typed.
	Path string
	// Dir is where the search started, for the --dir a remedy has to carry.
	Dir string
}

// FromManifest reports whether the id was read from a manifest rather than
// typed by the user.
func (r Ref) FromManifest() bool {
	return r.Source == WorkloadIDSourceManifest
}

// resolve returns the workload to act on: the positional id when one was given,
// otherwise the workloadId bound in the nearest .datarobot.yaml at or above
// dir. It is the pure half, so it tests without a command or a writer.
//
// An explicit id is returned without touching the filesystem, so a manifest
// that cannot be read is never a reason to refuse a command that was told
// which workload it means.
//
// The manifest is parsed but deliberately not validated: the ledger has no
// rule about workloadId, and logs is what you reach for when a project is
// broken.
func resolve(args []string, dir string) (Ref, error) {
	if len(args) > 0 {
		// Given-but-empty is an unset shell variable, not a request to fall
		// back: acting on the manifest would target a workload the caller
		// never named. Arity is the only thing that tells the two apart,
		// because "" and absent are the same string.
		id := strings.TrimSpace(args[0])
		if id == "" {
			return Ref{}, errors.New(
				"the workload id is empty. Pass an id, or omit it to use " + manifest.FileName)
		}

		return Ref{ID: id, Source: WorkloadIDSourceExplicit, Dir: dir}, nil
	}

	path, err := manifest.Locate(dir)
	if err != nil {
		if errors.Is(err, manifest.ErrNotFound) {
			// The sentinel leads rather than being nested: Locate says the same
			// thing in the same words, and a reader should not be told twice.
			// Where it looked is only worth naming when --dir sent it somewhere
			// other than where the reader is standing.
			where := "here or in any parent"
			if at := shortPath(dir); at != "." {
				where = "in " + at + " or any parent"
			}

			return Ref{}, fmt.Errorf("%w %s. Pass a workload id, or use --dir", manifest.ErrNotFound, where)
		}

		return Ref{}, err
	}

	m, err := manifest.Load(path)
	if err != nil {
		return Ref{}, fmt.Errorf("%w. Pass a workload id to skip reading it", err)
	}

	id := strings.TrimSpace(m.WorkloadID())
	if id == "" {
		return Ref{}, fmt.Errorf(
			"%s names no workload yet. Run 'dr workload up%s', or pass an id",
			shortPath(path), manifest.DirFlag(dir))
	}

	return Ref{ID: id, Source: WorkloadIDSourceManifest, Path: path, Dir: dir}, nil
}

// Wrap names the manifest behind a 404 or 403, which otherwise reports an id
// the caller never saw and no file. Other errors, and every error against a
// typed id, pass through untouched.
func (r Ref) Wrap(err error) error {
	if err == nil || !r.FromManifest() {
		return err
	}

	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	if httpErr.StatusCode != http.StatusNotFound && httpErr.StatusCode != http.StatusForbidden {
		return err
	}

	// A 404 means "not on this instance", which is not the same as gone: the
	// same manifest read against the wrong endpoint, organisation or token
	// answers exactly this way, so the remedy names both.
	if httpErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf("no access to workload %s, named by %s", r.ID, shortPath(r.Path))
	}

	return fmt.Errorf(
		"workload %s not found. %s names it; check the endpoint and organisation, or redeploy with 'dr workload up%s'",
		r.ID, shortPath(r.Path), manifest.DirFlag(r.Dir))
}

// shortPath renders a path the way a reader would type it: relative when that
// stays inside the working directory, absolute otherwise. Naming
// /Users/me/some/long/project/.datarobot.yaml at someone standing in it spends
// three lines saying "here".
func shortPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}

	rel, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}

	return rel
}
