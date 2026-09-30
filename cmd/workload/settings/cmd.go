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

// Package settings implements `dr workload settings`: read a workload's
// runtime (replicas, autoscaling, resources), or change it in place.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// Test seams. The command is mostly network, and the tests are not.
var (
	getSettingsFn     = workload.GetWorkloadSettings
	updateSettingsFn  = workload.UpdateWorkloadSettings
	guardFn           = workload.RefuseActiveReplacement
	waitReplacementFn = workload.WaitForReplacement
	waitWorkloadFn    = workload.WaitForWorkload
	readSpecFileFn    = workload.ReadSpecFile
)

// A settings change is a rolling replacement, which takes minutes rather
// than seconds, so the poll cadence is the deploy's rather than a build's.
const (
	settleInterval = 5 * time.Second
	settleTimeout  = 30 * time.Minute
)

// change is what the flags asked to change: one number on one group, or a
// whole settings body from a file.
type change struct {
	replicas    int
	hasReplicas bool
	group       string
	specFile    string
}

func (c change) requested() bool {
	return c.hasReplicas || c.specFile != ""
}

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		ref          idargs.Ref
		c            change
		poll         pollflags.Set
	)

	cmd := &cobra.Command{
		Use:   "settings [<workload-id>]",
		Short: "Show or change a workload's runtime settings.",
		Long: `Show or change how much a workload runs with: replicas, autoscaling,
resource bundles, and the CPU and memory of each container.

With no change flags the current settings are printed, one row per
container, with the replacement in flight when a change is still being
rolled out. This reads the same runtime that 'dr workload up' reconciles
from .datarobot.yaml, so it is the place to look at a workload that no
manifest describes.

--replicas N scales one container group. A workload with one group needs
no --group; with several, name it. A group that autoscales is refused,
because its count belongs to the autoscaler: change the policy instead,
with --spec-file. --spec-file <path> sends a whole settings body, JSON or
YAML, either {"runtime": ...} or the runtime block itself, which is the
shape 'dr workload settings <id> --output-format json' prints and the
shape a manifest carries under runtime.

A change is a rolling replacement: the platform brings up containers with
the new settings and retires the old ones, and the endpoint keeps
answering throughout. That is why the command confirms before changing,
and says so even when --yes or DATAROBOT_CLI_NON_INTERACTIVE skips the
question. A change is refused while another replacement is in flight.
Applied to a stopped workload, the platform starts it.

Without --wait the command returns once the platform has accepted the
change and names the replacement to follow. With --wait it follows the
replacement to its end, waits for the workload to be running on the new
settings, and prints them. A replacement that ends failed leaves the
workload on the settings it had, and the command says so and exits
non-zero.

JSON output is one {"settings": ...} document: the runtime in the
platform's own field names, the replacement, and for a change the status
"requested" or "applied".

` + idargs.HelpText + `

Example:
  dr workload settings
  dr workload settings 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload settings 68b0c1d2e3f4a5b6c7d8e9f0 --replicas 3
  dr workload settings 68b0c1d2e3f4a5b6c7d8e9f0 --replicas 3 --group default --wait
  dr workload settings 68b0c1d2e3f4a5b6c7d8e9f0 --spec-file settings.yaml --yes
  dr workload settings 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			c.hasReplicas = cmd.Flags().Changed("replicas")

			if err := c.check(); err != nil {
				return err
			}

			// After the flag checks, so a contradiction between flags is
			// not reported as a missing manifest.
			var err error

			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			if !c.requested() {
				return show(cmd, outputFormat, ref)
			}

			return apply(cmd, outputFormat, ref, c, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)
	idargs.AddYesFlag(cmd, "Skip the confirmation before a change. The rolling restart is still announced.")

	cmd.Flags().IntVar(&c.replicas, "replicas", 0, "Scale one container group to this many replicas.")
	cmd.Flags().StringVar(&c.group, "group", "",
		"The container group --replicas applies to. Needed only when the workload has more than one.")
	cmd.Flags().StringVar(&c.specFile, "spec-file", "",
		"A settings body to apply, JSON or YAML: {\"runtime\": ...} or the runtime block itself.")

	pollflags.RegisterWithDefaults(cmd, &poll, settleInterval, settleTimeout,
		"Wait until the new settings are rolled out and the workload is running on them.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"replicas":           c.hasReplicas,
			"spec_file":          c.specFile != "",
			"wait":               poll.Wait,
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}

// check refuses the flag combinations that cannot mean one thing.
func (c change) check() error {
	if c.hasReplicas && c.specFile != "" {
		return errors.New("--replicas and --spec-file are exclusive: one number on one group, or a whole settings body")
	}

	if c.hasReplicas && c.replicas < 0 {
		return fmt.Errorf("--replicas %d: a replica count cannot be negative", c.replicas)
	}

	if c.group != "" && !c.hasReplicas {
		return errors.New("--group says which container group --replicas applies to; it does nothing on its own")
	}

	return nil
}

// show prints the settings as they are.
func show(cmd *cobra.Command, format outputformat.OutputFormat, ref idargs.Ref) error {
	settings, err := getSettingsFn(ref.ID)
	if err != nil {
		return ref.Wrap(err)
	}

	return render(cmd.OutOrStdout(), format, settingsOutput{
		WorkloadID:  ref.ID,
		Runtime:     settings.Runtime,
		Replacement: settings.Replacement,
	})
}

// apply builds the runtime to send, confirms, starts the replacement and,
// with --wait, follows it to the workload running on the new settings.
func apply(cmd *cobra.Command, format outputformat.OutputFormat, ref idargs.Ref, c change, poll pollflags.Set) error {
	runtime, err := payloadFor(ref, c)
	if err != nil {
		return err
	}

	confirmed, err := idargs.Confirm(cmd,
		idargs.Prompt("Update the runtime settings of", ref, "The change rolls its containers onto the new settings."),
		idargs.EnvMayConsent)
	if err != nil || !confirmed {
		return err
	}

	// Said whether or not a question was asked: the restart is the part
	// worth knowing about, and --yes skips the question, not the fact. Not
	// under JSON, where stderr is kept clear for `2>&1 | jq .`.
	if format != outputformat.OutputFormatJSON {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Applying the settings rolls the workload's containers; the endpoint keeps answering "+
				"from the previous generation until the new one is ready.")
	}

	if err := guardFn(ref.ID); err != nil {
		if errors.Is(err, workload.ErrReplacementInFlight) {
			return err
		}

		return ref.Wrap(fmt.Errorf("cannot check for a rollout in flight: %w", err))
	}

	started, err := updateSettingsFn(ref.ID, runtime)
	if err != nil {
		return ref.Wrap(fmt.Errorf("cannot update the runtime settings: %w", err))
	}

	if !poll.Wait {
		if format != outputformat.OutputFormatJSON {
			fmt.Fprintln(cmd.ErrOrStderr(), "Check progress with: dr workload status "+ref.ID)
		}

		return render(cmd.OutOrStdout(), format, settingsOutput{
			WorkloadID:  ref.ID,
			Status:      "requested",
			Replacement: started,
		})
	}

	return follow(cmd, format, ref, started, poll)
}

// payloadFor is the runtime block the change asks to send.
func payloadFor(ref idargs.Ref, c change) (json.RawMessage, error) {
	if c.specFile != "" {
		doc, err := readSpecFileFn(c.specFile)
		if err != nil {
			return nil, err
		}

		return workload.RuntimeFromSettingsFile(doc)
	}

	settings, err := getSettingsFn(ref.ID)
	if err != nil {
		return nil, ref.Wrap(err)
	}

	runtime, err := settings.Decode()
	if err != nil {
		return nil, ref.Wrap(err)
	}

	group, err := groupFor(runtime, c.group)
	if err != nil {
		return nil, err
	}

	payload, err := settings.WithReplicaCount(group, c.replicas)
	if errors.Is(err, workload.ErrGroupAutoscales) {
		return nil, fmt.Errorf("%w; --replicas sets a fixed count, so change or disable the autoscaling "+
			"policy with --spec-file instead", err)
	}

	return payload, err
}

// groupFor names the group --replicas applies to: the one given, the only
// one, or an error naming the choices.
func groupFor(runtime *workload.RuntimeSettings, group string) (string, error) {
	if group != "" {
		return group, nil
	}

	names := make([]string, 0, len(runtime.ContainerGroups))
	for _, g := range runtime.ContainerGroups {
		names = append(names, g.Name)
	}

	switch len(names) {
	case 0:
		return "", errors.New("the workload has no container groups to scale")
	case 1:
		return names[0], nil
	default:
		return "", fmt.Errorf("the workload has %d container groups (%s): say which with --group",
			len(names), strings.Join(names, ", "))
	}
}

// follow waits for the replacement and then for the workload, and prints
// the settings as they are once it is running on them.
func follow(cmd *cobra.Command, format outputformat.OutputFormat, ref idargs.Ref,
	started *workload.Replacement, poll pollflags.Set,
) error {
	progress := func(msg string) {
		if format != outputformat.OutputFormatJSON {
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
		}
	}

	var last string

	final, err := waitReplacementFn(ref.ID, started, poll.Interval, poll.Timeout, func(r *workload.Replacement) {
		if r != nil && r.Status != last {
			last = r.Status

			progress("Replacement " + r.ID + ": " + r.Status)
		}
	})
	if err != nil {
		if final != nil && workload.IsFailedReplacementStatus(final.Status) {
			return fmt.Errorf("the settings update for workload %s ended as %s, so it is still running with the "+
				"settings it had; check 'dr workload status %s': %w", ref.ID, final.Status, ref.ID, err)
		}

		return ref.Wrap(err)
	}

	progress("Waiting for workload " + ref.ID + " to run on the new settings")

	if _, err := waitWorkloadFn(ref.ID, workload.Serving{AwaitDrain: true}, poll.Interval, poll.Timeout, nil); err != nil {
		return ref.Wrap(err)
	}

	settings, err := getSettingsFn(ref.ID)
	if err != nil {
		return ref.Wrap(err)
	}

	return render(cmd.OutOrStdout(), format, settingsOutput{
		WorkloadID:  ref.ID,
		Status:      "applied",
		Runtime:     settings.Runtime,
		Replacement: final,
	})
}

// settingsOutput is the stable JSON shape, and what the text renderer reads.
type settingsOutput struct {
	WorkloadID  string                `json:"workloadId"`
	Status      string                `json:"status,omitempty"`
	Runtime     json.RawMessage       `json:"runtime,omitempty"`
	Replacement *workload.Replacement `json:"replacement"`
}

func render(w io.Writer, format outputformat.OutputFormat, out settingsOutput) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(w, "settings", out)
	}

	return printSettings(w, out)
}

func printSettings(w io.Writer, out settingsOutput) error {
	switch out.Status {
	case "requested":
		fmt.Fprintf(w, "Settings update requested for workload %s", out.WorkloadID)

		if out.Replacement != nil {
			fmt.Fprintf(w, " (replacement %s)", out.Replacement.ID)
		}

		fmt.Fprintln(w)

		return nil
	case "applied":
		// The wait returns once the new generation is promoted, which can be
		// before the platform marks the replacement completed, so the
		// replacement is named here and not described as in flight below.
		fmt.Fprintf(w, "Settings applied; workload %s is running on them", out.WorkloadID)

		if out.Replacement != nil {
			fmt.Fprintf(w, " (replacement %s)", out.Replacement.ID)
		}

		fmt.Fprint(w, ".\n\n")
	}

	if len(out.Runtime) == 0 {
		return nil
	}

	var runtime workload.RuntimeSettings

	if err := json.Unmarshal(out.Runtime, &runtime); err != nil {
		return fmt.Errorf("cannot read the runtime settings: %w", err)
	}

	printRuntimeTable(w, runtime)

	if out.Status == "" {
		printReplacementLine(w, out.Replacement)
	}

	return nil
}

// printReplacementLine says what the settings route reported about a
// change in progress, if anything.
func printReplacementLine(w io.Writer, r *workload.Replacement) {
	if r == nil {
		return
	}

	if workload.IsTerminalReplacementStatus(r.Status) {
		fmt.Fprintf(w, "Last settings change: replacement %s (%s)\n", r.ID, r.Status)

		return
	}

	fmt.Fprintf(w, "A settings change is being rolled out: replacement %s (%s)\n", r.ID, r.Status)
}

func printRuntimeTable(w io.Writer, runtime workload.RuntimeSettings) {
	t := newTable("GROUP", "REPLICAS", "AUTOSCALING", "BUNDLE", "CONTAINER", "CPU", "MEMORY", "GPU")

	for _, g := range runtime.ContainerGroups {
		if len(g.Containers) == 0 {
			t.Row(g.Name, replicasCell(g), autoscalingCell(g), bundleCell(g), "-", "-", "-", "-")

			continue
		}

		for i, c := range g.Containers {
			group, replicas, autoscaling, bundle := g.Name, replicasCell(g), autoscalingCell(g), bundleCell(g)
			if i > 0 {
				group, replicas, autoscaling, bundle = "", "", "", ""
			}

			t.Row(group, replicas, autoscaling, bundle, c.Name,
				number(c.ResourceAllocation.CPU), memoryCell(c.ResourceAllocation.Memory), gpuCell(c.ResourceAllocation.GPU))
		}
	}

	fmt.Fprintln(w, t.String())
}

func replicasCell(g workload.ContainerGroupSettings) string {
	switch {
	case g.Autoscales():
		return "auto"
	case g.ReplicaCount == nil:
		return "-"
	default:
		return strconv.Itoa(*g.ReplicaCount)
	}
}

func autoscalingCell(g workload.ContainerGroupSettings) string {
	if !g.Autoscales() {
		return "-"
	}

	bounds := intCell(g.Autoscaling.MinReplicaCount) + "-" + intCell(g.Autoscaling.MaxReplicaCount)

	policies := make([]string, 0, len(g.Autoscaling.Policies))
	for _, p := range g.Autoscaling.Policies {
		policies = append(policies, p.ScalingMetric+"="+number(p.Target))
	}

	if len(policies) == 0 {
		return bounds
	}

	return bounds + " on " + strings.Join(policies, ", ")
}

// bundleCell is the bundle the platform resolved, or the one asked for
// while it has not answered, or nothing.
func bundleCell(g workload.ContainerGroupSettings) string {
	if g.ResolvedBundle != nil && g.ResolvedBundle.ID != "" {
		return g.ResolvedBundle.ID
	}

	if len(g.ResourceBundles) > 0 {
		return strings.Join(g.ResourceBundles, ", ")
	}

	return "-"
}

func intCell(n *int) string {
	if n == nil {
		return "?"
	}

	return strconv.Itoa(*n)
}

func gpuCell(gpu *float64) string {
	if gpu == nil || *gpu == 0 {
		return "-"
	}

	return number(*gpu)
}

func number(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
