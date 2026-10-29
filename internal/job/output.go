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

package job

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/outputformat"
)

const (
	timestampFormat       = "2006-01-02 15:04:05 UTC"
	emptyValuePlaceholder = "—"
)

// out is where renderers write; tests swap it.
var out io.Writer = os.Stdout

func orPlaceholder(value string) string {
	if value == "" {
		return emptyValuePlaceholder
	}

	return value
}

func timestamp(t *time.Time) string {
	if t == nil || t.IsZero() {
		return emptyValuePlaceholder
	}

	return t.UTC().Format(timestampFormat)
}

// RenderJob prints one job.
func RenderJob(format outputformat.OutputFormat, job Job) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(job)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", job.ID)
	fmt.Fprintf(w, "Name:\t%s\n", job.Name)
	fmt.Fprintf(w, "Description:\t%s\n", orPlaceholder(job.Description))
	fmt.Fprintf(w, "Artifact ID:\t%s\n", job.ArtifactID)
	fmt.Fprintf(w, "Created:\t%s\n", timestamp(&job.CreatedAt))
	fmt.Fprintf(w, "Updated:\t%s\n", timestamp(&job.UpdatedAt))

	return w.Flush()
}

// RenderJobs prints a job list.
func RenderJobs(format outputformat.OutputFormat, jobs []Job) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(out, "jobs", nonNil(jobs))
	}

	if len(jobs) == 0 {
		fmt.Fprintln(out, "No jobs found.")

		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "ID\tNAME\tARTIFACT ID\tUPDATED")

	for _, job := range jobs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", job.ID, job.Name, job.ArtifactID, timestamp(&job.UpdatedAt))
	}

	return w.Flush()
}

// RenderRun prints one run, with how it ended once it has.
func RenderRun(format outputformat.OutputFormat, run Run) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(run)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Run ID:\t%s\n", run.ID)
	fmt.Fprintf(w, "Job ID:\t%s\n", run.JobID)
	fmt.Fprintf(w, "Status:\t%s\n", run.Status)
	fmt.Fprintf(w, "Artifact ID:\t%s\n", run.ArtifactID)
	fmt.Fprintf(w, "Placement:\t%s\n", placement(run.Placement))
	fmt.Fprintf(w, "Created:\t%s\n", timestamp(&run.CreatedAt))
	fmt.Fprintf(w, "Started:\t%s\n", timestamp(run.StartedAt))
	fmt.Fprintf(w, "Finished:\t%s\n", timestamp(run.FinishedAt))

	if run.Ended() || run.ExitCode != nil {
		fmt.Fprintf(w, "Exit code:\t%s\n", exitCode(run.ExitCode))
		fmt.Fprintf(w, "Reason:\t%s\n", orPlaceholder(run.TerminationReason))
	}

	if run.TerminationMessage != "" {
		fmt.Fprintf(w, "Message:\t%s\n", run.TerminationMessage)
	}

	return w.Flush()
}

// RenderRuns prints a run list.
func RenderRuns(format outputformat.OutputFormat, runs []Run) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(out, "runs", nonNil(runs))
	}

	if len(runs) == 0 {
		fmt.Fprintln(out, "No runs found.")

		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "RUN ID\tSTATUS\tEXIT CODE\tREASON\tSTARTED\tFINISHED")

	for _, run := range runs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			run.ID, run.Status, exitCode(run.ExitCode), orPlaceholder(run.TerminationReason),
			timestamp(run.StartedAt), timestamp(run.FinishedAt))
	}

	return w.Flush()
}

func placement(p *Placement) string {
	switch {
	case p == nil || (p.Enclave == "" && p.EnclaveID == ""):
		return "Hub"
	case p.Enclave != "":
		return "Enclave " + p.Enclave
	default:
		return "Enclave " + p.EnclaveID
	}
}

func exitCode(code *int) string {
	if code == nil {
		return emptyValuePlaceholder
	}

	return strconv.Itoa(*code)
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintln(out, string(data))

	return nil
}

// nonNil keeps an empty list a JSON [] rather than null.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}
