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

package workload

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

// Diagnosis is why a workload is in the state it is in, read off every
// container generation the platform lists for it.
//
// `dr workload status` says "errored" and `dr workload logs` can legitimately
// say nothing at all: container stdout is gathered by a collector that is
// rolled out per cluster, and a container that never started wrote nothing
// anyway. The per-replica status details are where the platform keeps the
// reason, the restart count and the exit code, and nothing else in the CLI
// showed them.
type Diagnosis struct {
	WorkloadID  string                `json:"workloadId"`
	Name        string                `json:"name"`
	Status      string                `json:"status"`
	Generations []GenerationDiagnosis `json:"generations"`
}

// GenerationDiagnosis is one generation's snapshot and what stands out in it.
// Details is nil when the platform's monitor has not reported yet, which the
// route answers with 204 rather than an empty document.
type GenerationDiagnosis struct {
	ID         string               `json:"id"`
	ArtifactID string               `json:"artifactId"`
	Status     string               `json:"status"`
	Role       string               `json:"role,omitempty"`
	Details    *ProtonStatusDetails `json:"details"`
	Findings   []string             `json:"findings"`

	// Error is why this generation's status details could not be read,
	// empty when they were. Only a generation that is not the active one
	// carries it: the active generation's details are what the diagnosis
	// is for, so a failure there fails the whole read instead.
	Error string `json:"error,omitempty"`
}

// ErrGenerationsUnavailable reports an install without the protons route.
// It stands in for the platform's 404 rather than wrapping it, so a caller
// matching HTTP errors does not read it as the workload being missing.
var ErrGenerationsUnavailable = errors.New("this platform does not expose the container generations")

// Diagnose reads the workload, its generations and each generation's status
// details. The generation marked active comes first, because it is the one
// answering the endpoint; the others are what a rolling replacement leaves
// behind and are listed after it.
//
// An errored workload is an answer, not a failure: the command exists to
// explain that state, so only a read that could not be made is an error.
func Diagnose(workloadID string) (*Diagnosis, error) {
	wl, err := GetWorkload(workloadID)
	if err != nil {
		return nil, err
	}

	protons, err := ListProtons(workloadID)
	if err != nil {
		// The route is absent on some installs. That is not a broken read,
		// it is a platform with nothing to say, and the sentence should be
		// about that rather than about HTTP.
		var httpErr *drapi.HTTPError

		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf(
				"%w of workload %s, so there is nothing to diagnose here; 'dr workload get %s' has the status",
				ErrGenerationsUnavailable, workloadID, workloadID)
		}

		return nil, fmt.Errorf("cannot list the container generations of workload %s: %w", workloadID, err)
	}

	d := &Diagnosis{
		WorkloadID:  wl.ID,
		Name:        wl.Name,
		Status:      wl.Status,
		Generations: make([]GenerationDiagnosis, 0, len(protons)),
	}

	stopped := wl.Status == WorkloadStatusStopped

	for _, p := range activeFirst(protons) {
		g := GenerationDiagnosis{ID: p.ID, ArtifactID: p.ArtifactID, Status: p.Status, Role: p.Role, Findings: []string{}}

		details, err := GetProtonStatusDetails(workloadID, p.ID)

		switch {
		case err != nil && IsActiveProtonRole(p.Role):
			// The active generation is the one answering the endpoint and
			// the one the user came for; without its details there is no
			// diagnosis to give.
			return nil, fmt.Errorf("cannot read the status details of generation %s: %w", p.ID, err)
		case err != nil:
			// A draining generation can be collected between the list and
			// this read, or refused on its own. That is a note on it, not a
			// reason to throw away the active generation's answer.
			g.Error = err.Error()
		default:
			g.Details = details
			g.Findings = findings(p.ID, details, stopped)
		}

		d.Generations = append(d.Generations, g)
	}

	return d, nil
}

// startupReasons are the waiting reasons a container passes through on its
// way up. During a rolling replacement, which is what this command is for,
// the new generation shows them for a while; they are progress, not faults.
var startupReasons = []string{"ContainerCreating", "PodInitializing"}

// starting reports a container that is still coming up and has not failed
// before: a startup reason with no previous run to name.
func (c ContainerStatus) starting() bool {
	return slices.Contains(startupReasons, c.Reason) && c.lastExit() == ""
}

// activeFirst orders the generation marked active ahead of the rest, keeping
// the platform's order otherwise. A copy, so the caller's slice is untouched.
func activeFirst(protons []Proton) []Proton {
	ordered := slices.Clone(protons)

	slices.SortStableFunc(ordered, func(a, b Proton) int {
		switch {
		case IsActiveProtonRole(a.Role) == IsActiveProtonRole(b.Role):
			return 0
		case IsActiveProtonRole(a.Role):
			return -1
		default:
			return 1
		}
	})

	return ordered
}

// findings is what stands out in a snapshot, one line per container that
// has something to say: a waiting reason (CrashLoopBackOff, ImagePullBackOff,
// ErrImagePull), a previous run that ended badly (a non-zero exit, OOMKilled),
// or restarts on a container that is otherwise quiet. The lines are the ones
// `dr workload up` prints beside an errored state, so the two agree.
//
// A replica that never got as far as running a container is heard too: a
// pod the scheduler could not place has no container statuses at all, and
// the reason (Unschedulable, with the cluster's message about what was short)
// sits on its conditions.
//
// Never nil, so the JSON envelope carries [] rather than null for a
// generation with nothing wrong: a consumer can tell "checked, clean" from
// "not checked".
func findings(protonID string, details *ProtonStatusDetails, stopped bool) []string {
	out := []string{}

	// A stopped workload's containers ended because they were told to. A
	// Completed, or an Error with the exit code of a process that did not
	// trap the signal, is the whole story there rather than a fault, and
	// naming it would make every clean stop read as a finding.
	if details == nil || stopped {
		return out
	}

	for _, replica := range details.Replicas {
		out = append(out, replicaFindings(protonID, replica)...)

		for _, c := range replica.Containers {
			if c.starting() {
				continue
			}

			if line := c.failure(protonID); line != "" {
				out = append(out, line)

				continue
			}

			if c.RestartCount > 0 {
				out = append(out, fmt.Sprintf("%s: restarted %d %s", shortContainerName(protonID, c.Name),
					c.RestartCount, plural(c.RestartCount, "time", "times")))
			}
		}
	}

	return out
}

// benignConditionReasons are the condition reasons that say nothing a
// container line does not say better, or nothing at all. ContainersNotReady
// is the pod repeating what its containers already report; PodCompleted is
// how a clean stop reads on the pod.
var benignConditionReasons = []string{"", "ContainersNotReady", "PodCompleted"}

// replicaFindings is what the pod's own conditions say went wrong, which is
// the only account there is of a replica that never ran a container: the
// scheduler's verdict on one it could not place.
func replicaFindings(protonID string, replica ReplicaStatus) []string {
	var out []string

	for _, cond := range replica.Conditions {
		if cond.Value || slices.Contains(benignConditionReasons, cond.Reason) {
			continue
		}

		line := shortContainerName(protonID, replica.Name) + ": " + cond.Reason
		if cond.Message != "" {
			line += ": " + cond.Message
		}

		out = append(out, line)
	}

	return out
}

// RenderDiagnosis prints a diagnosis to stdout in the requested format.
func RenderDiagnosis(format outputformat.OutputFormat, d Diagnosis) error {
	return RenderDiagnosisTo(os.Stdout, format, d)
}

// RenderDiagnosisTo is RenderDiagnosis with the writer chosen by the caller,
// so a command can hand it the stream cobra gave it and a test can read it
// back. Under JSON the writer receives exactly one document and nothing else.
func RenderDiagnosisTo(w io.Writer, format outputformat.OutputFormat, d Diagnosis) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(w, "diagnosis", d)
	}

	printDiagnosis(w, d)

	return nil
}

func printDiagnosis(w io.Writer, d Diagnosis) {
	fmt.Fprintf(w, "Workload %s (%s) is %s\n", d.WorkloadID, d.Name, statusStyle(d.Status).Render(d.Status))

	if len(d.Generations) == 0 {
		fmt.Fprintln(w, tui.HintStyle.Render(
			"The platform lists no container generations for it, so there is nothing to diagnose yet."))

		return
	}

	for _, g := range d.Generations {
		fmt.Fprintln(w)
		printGeneration(w, g)
	}
}

func printGeneration(w io.Writer, g GenerationDiagnosis) {
	title := "Generation " + g.ID + " · " + g.Status
	if IsActiveProtonRole(g.Role) {
		title += " · active"
	}

	fmt.Fprintf(w, "%s  %s\n", tui.InfoStyle.Render(title), tui.DimStyle.Render("artifact "+g.ArtifactID))

	if g.Error != "" {
		fmt.Fprintf(w, "  %s\n", tui.WarnStyle.Render("Status details could not be read: "+g.Error))

		return
	}

	if g.Details == nil {
		fmt.Fprintf(w, "  %s\n", tui.HintStyle.Render(
			"No status snapshot yet: the platform's monitor has not reported for this generation."))

		return
	}

	if summary := g.Details.OverallStatus.Summary; summary != "" {
		fmt.Fprintf(w, "  %s\n", tui.DimStyle.Render(summary))
	}

	printReplicaTable(w, g.ID, g.Details.Replicas)

	if len(g.Findings) == 0 {
		fmt.Fprintf(w, "  %s\n", tui.HintStyle.Render("No container is reporting a problem."))

		return
	}

	fmt.Fprintln(w, "  Findings:")

	for _, line := range g.Findings {
		fmt.Fprintf(w, "    %s %s\n", tui.WarnStyle.Render("⚠"), line)
	}
}

// printReplicaTable is one row per container, grouped under its replica. The
// replica and container names drop the cluster's lrs-<generation>- prefix,
// which every name in a generation shares and which says nothing.
func printReplicaTable(w io.Writer, protonID string, replicas []ReplicaStatus) {
	if len(replicas) == 0 {
		fmt.Fprintf(w, "  %s\n", tui.HintStyle.Render("No replicas are reported for this generation."))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			return cellStyle
		}).
		Headers("REPLICA", "CONTAINER", "STATE", "REASON", "RESTARTS", "READY", "LAST EXIT")

	for _, r := range replicas {
		replica := shortContainerName(protonID, r.Name) + " (" + orPlaceholder(r.Status) + ")"

		if len(r.Containers) == 0 {
			t.Row(replica, "-", "-", "-", "-", "-", "-")

			continue
		}

		for _, c := range r.Containers {
			t.Row(
				replica,
				shortContainerName(protonID, c.Name),
				orPlaceholder(c.Status),
				orPlaceholder(c.Reason),
				strconv.Itoa(c.RestartCount),
				yesNo(c.Ready),
				lastExitCell(c),
			)
		}
	}

	for line := range strings.SplitSeq(t.String(), "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
}

// lastExitCell is how the container's most recent run ended: the previous
// state when the cluster recorded one, otherwise the container's own exit
// code when it is terminated right now. The reason rides along when the
// cluster gave one, since "exit 137 (OOMKilled)" says more than the number.
func lastExitCell(c ContainerStatus) string {
	code, reason := c.ExitCode, ""

	if c.LastState != nil && c.LastState.ExitCode != nil {
		code, reason = c.LastState.ExitCode, c.LastState.Reason
	} else if c.LastState == nil && c.ExitCode != nil {
		reason = c.Reason
	}

	if code == nil {
		return "-"
	}

	cell := "exit " + strconv.Itoa(*code)

	if reason != "" {
		cell += " (" + reason + ")"
	}

	return cell
}

// shortContainerName drops the cluster's lrs-<generation>- prefix, the way
// every message in this package names a container.
func shortContainerName(protonID, name string) string {
	short := strings.TrimPrefix(name, "lrs-"+protonID+"-")
	if short == "" {
		return name
	}

	return short
}

func statusStyle(status string) lipgloss.Style {
	switch {
	case IsWorkloadErrorStatus(status):
		return tui.ErrorStyle
	case strings.EqualFold(status, WorkloadStatusRunning):
		return tui.SuccessStyle
	default:
		return tui.WarnStyle
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}

	return "no"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}
