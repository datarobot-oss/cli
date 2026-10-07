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

// Package promote implements `dr workload promote`: lock the draft artifact a
// workload is running, in place, so it becomes a permanent version.
package promote

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// Seams so tests stay off the network.
var (
	promoteWorkloadFn = workload.PromoteWorkload
	getArtifactFn     = workload.GetArtifact
)

// Output is the JSON document --output-format json prints.
type Output struct {
	WorkloadID string `json:"workloadId"`
	ArtifactID string `json:"artifactId"`
	Version    *int   `json:"version"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var ref idargs.Ref

	cmd := &cobra.Command{
		Use:   "promote [<workload-id>]",
		Short: "Make the version a workload is running permanent.",
		Long: `Make the version a workload is running permanent.

The draft artifact the workload is serving is locked in place and given a
version number. The workload keeps running it; nothing is rebuilt or rolled.
Locking is one-way: a locked artifact cannot be changed, so the next deploy
of this workload is a new version rather than an edit to this one. A locked
version can be shared with other workloads by naming its artifact id in
their spec.

A workload that is already running a locked version, one with no running
generation, or one with a rollout in flight is refused with the platform's
reason. Only the artifact's owner can promote it.

` + idargs.HelpText + `

A workload whose id is specified in the manifest rather than on the
command line is confirmed first; only --yes skips that. Locking cannot be
undone, so the environment variable that suppresses wizards in CI is not
taken as consent to lock something nobody named, as with delete.

Example:
  dr workload promote
  dr workload promote 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload promote 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			var err error

			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			// Only an ambient target is confirmed: a typed id is the consent.
			if ref.FromManifest() {
				confirmed, err := idargs.Confirm(cmd,
					idargs.Prompt("Promote", ref, "Locking is permanent."), idargs.EnvMayNotConsent)
				if err != nil || !confirmed {
					return err
				}
			}

			w, err := promoteWorkloadFn(ref.ID)
			if err != nil {
				return promoteError(err, ref)
			}

			return render(cmd, outputFormat, w)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)
	idargs.AddYesFlag(cmd, "Skip the confirmation asked when the id is specified in the manifest.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}

// promoteError keeps the provenance of the id, except that this route's 404
// also covers an artifact the caller does not own, which the usual "not on
// this instance" wording would send the reader to check their endpoint for.
func promoteError(err error, ref idargs.Ref) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return fmt.Errorf("cannot promote workload %s%s: the platform has no such workload, or its artifact is not "+
			"yours to lock (only the artifact's owner can promote it): %w", ref.ID, ref.SpecifiedIn(), err)
	}

	return ref.Wrap(err)
}

// render reports the promotion. The promote route answers with the workload,
// which carries no version number, so the artifact is read back for it; a
// read that fails leaves the version out rather than failing a promotion that
// already happened.
func render(cmd *cobra.Command, format outputformat.OutputFormat, w *workload.Workload) error {
	out := Output{WorkloadID: w.ID, ArtifactID: w.ArtifactID}

	if artifact, err := getArtifactFn(w.ArtifactID); err == nil {
		out.Version = artifact.Version
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "Promoted, but the version could not be read back: %v\n", err)
	}

	if format == outputformat.OutputFormatJSON {
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	version := ""
	if out.Version != nil {
		version = fmt.Sprintf(" as version %d", *out.Version)
	}

	fmt.Println(tui.BaseTextStyle.Render(
		fmt.Sprintf("Promoted workload %s: artifact %s is locked%s.", out.WorkloadID, out.ArtifactID, version)))

	return nil
}
