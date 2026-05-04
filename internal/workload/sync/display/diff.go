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

package display

import (
	"fmt"
	"io"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// ContentFetcher returns the local and remote bytes for a path.
type ContentFetcher interface {
	LocalContent(path string) ([]byte, error)
	RemoteContent(path string) ([]byte, error)
}

const devNull = "/dev/null"

// PrintDiffs writes a per-file unified-style diff for every plan entry
// that has bytes worth showing. Fetcher errors are rendered inline so a
// single missing object does not suppress the rest of the report.
func PrintDiffs(w io.Writer, plan *sync.SyncPlan, fetcher ContentFetcher) error {
	if plan == nil || fetcher == nil {
		return nil
	}

	dmp := diffmatchpatch.New()

	rows := make([]sync.FileAction, 0, len(plan.Uploads)+len(plan.Downloads)+len(plan.Deletes)+len(plan.Conflicts))
	rows = append(rows, plan.Uploads...)
	rows = append(rows, plan.Downloads...)
	rows = append(rows, plan.Deletes...)
	rows = append(rows, plan.Conflicts...)

	for _, fa := range rows {
		if !shouldDiff(fa) {
			continue
		}

		if err := printOneDiff(w, fa, fetcher, dmp); err != nil {
			return err
		}
	}

	return nil
}

func shouldDiff(fa sync.FileAction) bool {
	switch fa.Classification {
	case sync.ClsLocalAdded, sync.ClsLocalModified, sync.ClsLocalDeleted,
		sync.ClsRemoteAdded, sync.ClsRemoteModified, sync.ClsRemoteDeleted,
		sync.ClsConflict, sync.ClsAddConflict,
		sync.ClsDelEditConflict, sync.ClsEditDelConflict:
		return true
	case sync.ClsUnchanged, sync.ClsConverged, sync.ClsBothDeleted, sync.ClsBothAddedSame:
		return false
	}

	return false
}

func printOneDiff(w io.Writer, fa sync.FileAction, fetcher ContentFetcher, dmp *diffmatchpatch.DiffMatchPatch) error {
	aHeader, bHeader := headerFor(fa)

	a, b, err := loadDiffPair(fa, fetcher)
	if err != nil {
		_, _ = fmt.Fprintf(w, "--- %s\n*** %s ***\n", fa.Path, err.Error())
		return nil
	}

	if _, err := fmt.Fprintf(w, "--- %s\n+++ %s\n", aHeader, bHeader); err != nil {
		return err
	}

	diffs := dmp.DiffMain(string(a), string(b), false)
	pretty := dmp.DiffPrettyText(diffs)

	if _, err := fmt.Fprint(w, pretty); err != nil {
		return err
	}

	if len(pretty) > 0 && pretty[len(pretty)-1] != '\n' {
		_, _ = fmt.Fprintln(w)
	}

	return nil
}

type headerLabels struct {
	left, right func(path string) string
}

var headerTable = map[sync.Classification]headerLabels{
	sync.ClsLocalAdded:      {staticLabel(devNull), suffixLabel(" (local; new file)")},
	sync.ClsLocalModified:   {suffixLabel(" (remote)"), suffixLabel(" (local)")},
	sync.ClsLocalDeleted:    {suffixLabel(" (remote)"), staticLabel(devNull)},
	sync.ClsRemoteAdded:     {staticLabel(devNull), suffixLabel(" (remote; new file)")},
	sync.ClsRemoteModified:  {suffixLabel(" (local)"), suffixLabel(" (remote)")},
	sync.ClsRemoteDeleted:   {suffixLabel(" (local)"), staticLabel(devNull)},
	sync.ClsConflict:        {suffixLabel(" (local)"), suffixLabel(" (remote, wins)")},
	sync.ClsAddConflict:     {suffixLabel(" (local)"), suffixLabel(" (remote, wins)")},
	sync.ClsDelEditConflict: {suffixLabel(" (local edit)"), staticLabel(devNull + " (remote deleted)")},
	sync.ClsEditDelConflict: {staticLabel(devNull + " (local deleted)"), suffixLabel(" (remote)")},
}

func headerFor(fa sync.FileAction) (string, string) {
	h, ok := headerTable[fa.Classification]
	if !ok {
		return fa.Path, fa.Path
	}

	return h.left(fa.Path), h.right(fa.Path)
}

func staticLabel(s string) func(string) string { return func(string) string { return s } }
func suffixLabel(suffix string) func(string) string {
	return func(path string) string { return path + suffix }
}

// loadDiffPair returns the (before, after) byte pair for a classification.
// A nil slice on either side means that side has no content; DiffMain
// treats it as empty and produces all-add or all-delete output.
func loadDiffPair(fa sync.FileAction, fetcher ContentFetcher) ([]byte, []byte, error) {
	switch fa.Classification {
	case sync.ClsLocalAdded:
		return localOnly(fa, fetcher, false)
	case sync.ClsLocalModified, sync.ClsConflict, sync.ClsAddConflict:
		return localAndRemote(fa, fetcher, false)
	case sync.ClsLocalDeleted:
		return remoteOnly(fa, fetcher, true)
	case sync.ClsRemoteAdded:
		return remoteOnly(fa, fetcher, false)
	case sync.ClsRemoteModified:
		return localAndRemote(fa, fetcher, true)
	case sync.ClsRemoteDeleted:
		return localOnly(fa, fetcher, true)
	case sync.ClsDelEditConflict:
		return localOnly(fa, fetcher, true)
	case sync.ClsEditDelConflict:
		return remoteOnly(fa, fetcher, false)
	case sync.ClsUnchanged, sync.ClsConverged, sync.ClsBothDeleted, sync.ClsBothAddedSame:
		return nil, nil, nil
	}

	return nil, nil, fmt.Errorf("unsupported classification for diff: %s", fa.Classification)
}

func localOnly(fa sync.FileAction, fetcher ContentFetcher, asBefore bool) ([]byte, []byte, error) {
	local, err := fetcher.LocalContent(fa.Path)
	if err != nil {
		return nil, nil, err
	}

	if asBefore {
		return local, nil, nil
	}

	return nil, local, nil
}

func remoteOnly(fa sync.FileAction, fetcher ContentFetcher, asBefore bool) ([]byte, []byte, error) {
	remote, err := fetcher.RemoteContent(fa.Path)
	if err != nil {
		return nil, nil, err
	}

	if asBefore {
		return remote, nil, nil
	}

	return nil, remote, nil
}

func localAndRemote(fa sync.FileAction, fetcher ContentFetcher, localFirst bool) ([]byte, []byte, error) {
	local, err := fetcher.LocalContent(fa.Path)
	if err != nil {
		return nil, nil, err
	}

	remote, err := fetcher.RemoteContent(fa.Path)
	if err != nil {
		return nil, nil, err
	}

	if localFirst {
		return local, remote, nil
	}

	return remote, local, nil
}
