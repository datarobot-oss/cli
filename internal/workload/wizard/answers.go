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

package wizard

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/datarobot/cli/internal/workload/manifest"
)

// Answers is what the flags already said. Every field that is set answers its
// question and skips that screen; what is left over is either asked or, with
// no terminal, reported as a missing flag.
type Answers struct {
	// WorkloadID binds an existing workload. Exclusive with Name.
	WorkloadID string
	// Name names a new workload, created by the first up.
	Name string
	// Type is manifest.TypeService or manifest.TypeAgent.
	Type string
	// A2AEnabled publishes an agent's A2A card to the tenant-wide registry.
	A2AEnabled bool
	// BuildMode is one of the manifest.BuildMode* values.
	BuildMode string
	// Image is the published image URI, for BuildModeImage.
	Image string
	// ExecutionEnvironment names a base image by name or id, for
	// BuildModeGenerated. It is resolved to both ids before anything is
	// written.
	ExecutionEnvironment string
	// Entrypoint is the raw command line, parsed shell-style so quoted
	// arguments survive.
	Entrypoint string
	Port       int
	HealthPath string
	// NoProbe writes no readinessProbe block at all. Declining is a separate
	// field rather than an empty HealthPath because this struct is the wizard's
	// own vocabulary and is filled in from more than one place: "" here means
	// "nobody said", and a caller that never touches cobra has no other way to
	// say "no probe" out loud.
	NoProbe  bool
	Replicas int
	CPU      float64
	Memory   string
	// Importance is the scheduling priority, one of
	// manifest.ImportanceLevels.
	Importance string
	// SkipEnv leaves the project's .env out of the manifest entirely. By
	// default the variables are carried across, because a deploy that
	// silently drops the app's configuration is the worse default.
	SkipEnv bool
}

// ErrCancelled reports that the user backed out. Nothing is written and the
// command exits nonzero, so `dr workload config && dr workload up` stops
// here rather than deploying something nobody agreed to.
var ErrCancelled = errors.New("cancelled")

// check holds the flag combinations that cannot be resolved into a manifest,
// whether or not a terminal is available. Combinations that are merely
// incomplete are the headless path's business: interactively they are just
// questions that have not been answered yet.
func (a Answers) check() error {
	for _, check := range []func() error{
		a.checkBinding, a.checkKind, a.checkImageSource, a.checkReadiness, a.checkSizing, a.checkImportance,
	} {
		if err := check(); err != nil {
			return err
		}
	}

	return nil
}

func (a Answers) checkBinding() error {
	if a.WorkloadID != "" && a.Name != "" {
		return errors.New("--workload-id and --name are exclusive: bind an existing workload or name a new one")
	}

	return nil
}

func (a Answers) checkKind() error {
	if a.Type != "" && a.Type != manifest.TypeService && a.Type != manifest.TypeAgent {
		return fmt.Errorf("--type %q is not supported: use %s or %s",
			a.Type, manifest.TypeService, manifest.TypeAgent)
	}

	return nil
}

// checkA2A runs against the type that will be written rather than against the
// flag, and so it runs once the draft has one. Omitting --type does not turn
// --a2a-enabled into a service flag, it means whatever the draft started from
// stands: the documented default for a new workload, the live kind for a bound
// one. Checking the flag alone would reject --a2a-enabled on a bound agent
// unless the caller redundantly repeated --type agent.
func (a Answers) checkA2A(draftType string) error {
	if a.A2AEnabled && draftType != manifest.TypeAgent {
		return fmt.Errorf("--a2a-enabled applies to --type %s, not %s", manifest.TypeAgent, draftType)
	}

	return nil
}

func (a Answers) checkImageSource() error {
	switch a.BuildMode {
	case "", manifest.BuildModeDockerfile, manifest.BuildModeGenerated, manifest.BuildModeImage:
		return nil
	default:
		return fmt.Errorf("--build-mode %q is not supported: use %s, %s or %s",
			a.BuildMode, manifest.BuildModeDockerfile, manifest.BuildModeGenerated, manifest.BuildModeImage)
	}
}

// checkProbeExclusive is the one readiness rule no screen can settle, so it is
// the only one the interactive path refuses up front. The settings screen has a
// single health field and cannot represent "both were passed"; a path that
// simply lacks its leading slash, or an importance outside the enum, is a value
// the screen can show and the user can correct.
func (a Answers) checkProbeExclusive() error {
	if a.NoProbe && a.HealthPath != "" {
		return fmt.Errorf("--no-readiness-probe and --health %q are exclusive: "+
			"pass --health alone to probe that path, or --no-readiness-probe alone to run without a probe",
			a.HealthPath)
	}

	return nil
}

func (a Answers) checkReadiness() error {
	if err := a.checkProbeExclusive(); err != nil {
		return err
	}

	if a.HealthPath != "" && !strings.HasPrefix(a.HealthPath, "/") {
		return fmt.Errorf("--health %q must start with /", a.HealthPath)
	}

	if a.Port != 0 {
		if err := checkPort(a.Port, "--port"); err != nil {
			return err
		}
	}

	return nil
}

// checkPort holds both ends of the range. The floor is the ledger's, checked
// here too so a bad flag is rejected by the flag rather than by the file it
// would have produced; the ceiling is simply what a port number is.
func checkPort(port int, source string) error {
	switch {
	case port > maxPort:
		return fmt.Errorf("%s %d is not a port number: the highest is %d", source, port, maxPort)
	case port < minPort:
		return fmt.Errorf("%s %d is too low: the primary container runs unprivileged and must listen on %d or above",
			source, port, minPort)
	}

	return nil
}

// checkSizing holds the runtime numbers to what a replica can be. The
// platform would refuse these too, but only after a round trip, and a
// negative replica count written into a committed file is a confusing thing
// to find later.
func (a Answers) checkSizing() error {
	if a.Replicas < 0 {
		return fmt.Errorf("--replicas %d is negative", a.Replicas)
	}

	// ParseFloat accepts NaN and Inf, and neither compares as negative, so
	// without this they reach the file as `cpu: !!float NaN`, which the
	// manifest's own validator has no opinion about and the platform cannot
	// schedule.
	if math.IsNaN(a.CPU) || math.IsInf(a.CPU, 0) {
		return fmt.Errorf("--cpu %v is not a number of cores", a.CPU)
	}

	if a.CPU < 0 {
		return fmt.Errorf("--cpu %v is negative", a.CPU)
	}

	if a.Memory != "" && !manifest.ValidMemory(a.Memory) {
		return fmt.Errorf("--memory %q is not a valid size: use a byte count or a 1000-based unit (B, KB, MB, GB)",
			a.Memory)
	}

	return nil
}

// checkProbeEditable refuses a probe answer the CLI cannot carry out, which is
// only possible once the workload has been read.
//
// A readiness probe that declares no path is not the HTTP probe these flags
// describe, and binding preserves it rather than rewriting a shape this release
// does not model. Preserving is right when nobody said anything about the
// probe. When a flag did say something, staying silent would mean the run
// reports success having ignored the one thing it was asked to change, and
// without a screen there is nothing else to notice it.
func (a Answers) checkProbeEditable(live manifest.Live) error {
	if !a.NoProbe && a.HealthPath == "" {
		return nil
	}

	present, readable := live.ReadinessProbe()
	if !present || readable {
		return nil
	}

	flag := "--health " + a.HealthPath
	if a.NoProbe {
		flag = "--no-readiness-probe"
	}

	return fmt.Errorf(
		"%s runs a readiness probe with no path, which this CLI keeps as it is rather than rewriting a shape it "+
			"does not model, so %s cannot be applied to it. Drop the flag to bind without touching the probe, "+
			"or change the probe where it was defined",
		live.Name, flag)
}

// checkImportance holds the flag to the enum. The reader passes an
// unrecognized level through and lets the platform decide, but a level nobody
// meant to type is worth refusing here rather than after a round trip.
func (a Answers) checkImportance() error {
	if a.Importance != "" && !manifest.ValidImportance(a.importance()) {
		return fmt.Errorf("--importance %q is not supported: use one of %s",
			a.Importance, strings.Join(manifest.ImportanceLevels, ", "))
	}

	return nil
}

// importance is the level to write, lowercased because the casing is the
// user's business and the file's is settled.
func (a Answers) importance() string {
	return strings.ToLower(strings.TrimSpace(a.Importance))
}

// healthPath is the path to probe: the flag, the documented default, or
// nothing at all when the probe was declined.
func (a Answers) healthPath() string {
	if a.NoProbe {
		return ""
	}

	return orDefault(a.HealthPath, manifest.DefaultHealthPath)
}

// The range a primary container's port may fall in. The floor is the
// manifest ledger's; below it an unprivileged container cannot bind, so the
// workload never becomes ready.
const (
	minPort = 1024
	maxPort = 65535
)

// draft answers every question from the flags and the project, and reports
// what neither could answer. This is the headless path in full: no prompt is
// ever attempted, and the error names the flag that would have settled it.
func (a Answers) draft(detected Detected) (manifest.Draft, error) {
	if err := a.check(); err != nil {
		return manifest.Draft{}, err
	}

	name := a.Name
	if name == "" {
		name = detected.Name
	}

	draft := manifest.Draft{
		WorkloadID: a.WorkloadID,
		Name:       name,
		Importance: orDefault(a.importance(), manifest.DefaultImportance),
		Type:       manifest.ArtifactTypeOrDefault(a.Type),
		A2AEnabled: a.A2AEnabled,
		Port:       a.Port,
		HealthPath: a.healthPath(),
		Runtime:    a.runtime(),
		EnvVars:    a.envVars(detected),
	}

	if err := a.checkA2A(draft.Type); err != nil {
		return manifest.Draft{}, err
	}

	if draft.Port == 0 {
		draft.Port = detected.Port
	}

	build, err := a.build(detected)
	if err != nil {
		return manifest.Draft{}, err
	}

	draft.Build = build

	return draft, nil
}

// envVars is the .env listing the file will carry: everything the classifier
// found by default, none when the user opted out. Local-only variables are
// dropped, because deploying them would be wrong rather than unnecessary.
func (a Answers) envVars(detected Detected) []manifest.EnvVar {
	if a.SkipEnv {
		return nil
	}

	out := make([]manifest.EnvVar, 0, len(detected.EnvVars))

	for _, v := range detected.EnvVars {
		if v.Kind == EnvLocal {
			continue
		}

		out = append(out, manifest.EnvVar{
			Name:   v.Name,
			Value:  v.Value,
			Secret: v.Kind == EnvSecret,
		})
	}

	return out
}

func (a Answers) runtime() manifest.Runtime {
	runtime := manifest.Runtime{
		Replicas: a.Replicas,
		CPU:      a.CPU,
		Memory:   manifest.NormalizeMemory(orDefault(a.Memory, manifest.DefaultMemory)),
	}

	if runtime.Replicas == 0 {
		runtime.Replicas = manifest.DefaultReplicas
	}

	if runtime.CPU == 0 {
		runtime.CPU = manifest.DefaultCPU
	}

	return runtime
}

// build resolves the image source. With no --build-mode the mode follows what
// the flags imply and then what the project shows, so the Dockerfile track
// needs no flags at all when there is a Dockerfile to build.
func (a Answers) build(detected Detected) (manifest.Build, error) {
	switch a.buildMode(detected) {
	case manifest.BuildModeDockerfile:
		if !detected.HasDockerfile {
			return manifest.Build{}, fmt.Errorf("--build-mode %s needs a %s in %s",
				manifest.BuildModeDockerfile, DockerfileName, detected.Dir)
		}

		return manifest.Build{Mode: manifest.BuildModeDockerfile}, nil

	case manifest.BuildModeImage:
		if a.Image == "" {
			return manifest.Build{}, fmt.Errorf("--image is required with --build-mode %s", manifest.BuildModeImage)
		}

		return manifest.Build{Mode: manifest.BuildModeImage, ImageURI: a.Image}, nil

	case manifest.BuildModeGenerated:
		return a.generatedBuild()

	default:
		return manifest.Build{}, fmt.Errorf(
			"no %s found in %s, so the image source cannot be guessed: pass --build-mode %s with --image, "+
				"or --build-mode %s with --execution-environment and --entrypoint",
			DockerfileName, detected.Dir, manifest.BuildModeImage, manifest.BuildModeGenerated)
	}
}

// buildMode picks the mode the flags describe. An explicit --build-mode wins;
// otherwise a lone --image or --execution-environment says plainly enough
// which track the user is on, and a Dockerfile in the directory settles the
// rest. An empty return means nothing decided it.
func (a Answers) buildMode(detected Detected) string {
	switch {
	case a.BuildMode != "":
		return a.BuildMode
	case a.Image != "":
		return manifest.BuildModeImage
	case a.ExecutionEnvironment != "" || a.Entrypoint != "":
		return manifest.BuildModeGenerated
	case detected.HasDockerfile:
		return manifest.BuildModeDockerfile
	default:
		return ""
	}
}

func (a Answers) generatedBuild() (manifest.Build, error) {
	var missing []string

	if a.ExecutionEnvironment == "" {
		missing = append(missing, "--execution-environment")
	}

	if a.Entrypoint == "" {
		missing = append(missing, "--entrypoint")
	}

	if len(missing) > 0 {
		return manifest.Build{}, fmt.Errorf("%s %s required with --build-mode %s",
			strings.Join(missing, " and "), Plural(len(missing), "is", "are"), manifest.BuildModeGenerated)
	}

	entrypoint, err := splitCommand(a.Entrypoint)
	if err != nil {
		return manifest.Build{}, fmt.Errorf("--entrypoint %q: %w", a.Entrypoint, err)
	}

	// Resolved at setup, not at deploy: the file records both ids so the same
	// manifest builds the same image after the environment moves on, and a
	// name that does not exist fails now rather than after a sync.
	id, versionID, err := resolveExecEnvFn(a.ExecutionEnvironment)
	if err != nil {
		return manifest.Build{}, err
	}

	return manifest.Build{
		Mode:                          manifest.BuildModeGenerated,
		ExecutionEnvironmentID:        id,
		ExecutionEnvironmentVersionID: versionID,
		Entrypoint:                    entrypoint,
	}, nil
}

// splitCommand parses a command line the way a shell would, so quoted
// arguments survive as single arguments. It handles single quotes (literal),
// double quotes (backslash escapes honored) and backslash escapes outside
// quotes, which covers every entrypoint anyone writes; anything needing more
// than that belongs in a script the entrypoint calls.
func splitCommand(command string) ([]string, error) {
	var splitter commandSplitter

	for _, r := range command {
		splitter.step(r)
	}

	return splitter.done()
}

// commandSplitter is splitCommand's state: which argument is being built and
// what the last character did to the parse.
type commandSplitter struct {
	args    []string
	current strings.Builder
	// quote is the open quote character, 0 outside quotes.
	quote rune
	// escaped means the previous character was a backslash.
	escaped bool
	// started distinguishes an argument that is empty because nothing has
	// been read from one that is empty because it was written as "".
	started bool
}

func (s *commandSplitter) step(r rune) {
	switch {
	case s.escaped:
		s.current.WriteRune(r)

		// An escaped character starts an argument like any other. Without
		// this, an argument built only from escapes is dropped and its
		// characters leak into the next one.
		s.started = true
		s.escaped = false
	case s.quote != 0:
		s.inQuote(r)
	default:
		s.unquoted(r)
	}
}

// inQuote consumes a character inside quotes, where only the matching quote
// and, in double quotes, a backslash mean anything.
func (s *commandSplitter) inQuote(r rune) {
	switch {
	case r == s.quote:
		s.quote = 0
	case r == '\\' && s.quote == '"':
		s.escaped = true
	default:
		s.current.WriteRune(r)
	}
}

// unquoted consumes a character outside quotes, where whitespace separates
// arguments and a quote opens one.
func (s *commandSplitter) unquoted(r rune) {
	switch r {
	case '\\':
		s.escaped = true
	case '\'', '"':
		s.quote = r
		s.started = true
	case ' ', '\t', '\n':
		s.endArg()
	default:
		s.current.WriteRune(r)

		s.started = true
	}
}

func (s *commandSplitter) endArg() {
	if !s.started {
		return
	}

	s.args = append(s.args, s.current.String())
	s.current.Reset()
	s.started = false
}

func (s *commandSplitter) done() ([]string, error) {
	switch {
	case s.quote != 0:
		return nil, errors.New("unbalanced quote")
	case s.escaped:
		return nil, errors.New("trailing backslash")
	}

	s.endArg()

	if len(s.args) == 0 {
		return nil, errors.New("is empty")
	}

	return s.args, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
