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
	Replicas   int
	CPU        float64
	Memory     string
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
	for _, check := range []func() error{a.checkBinding, a.checkKind, a.checkImageSource, a.checkReadiness, a.checkSizing} {
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

	// The check is against the type that will be written, not against the
	// flag: omitting --type does not turn --a2a-enabled into a service flag,
	// it just means the default applies.
	if a.A2AEnabled && a.effectiveType() != manifest.TypeAgent {
		return fmt.Errorf("--a2a-enabled applies to --type %s, not %s",
			manifest.TypeAgent, a.effectiveType())
	}

	return nil
}

// effectiveType is the kind that will be written when nothing else changes it.
func (a Answers) effectiveType() string {
	return orDefault(a.Type, manifest.TypeService)
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

func (a Answers) checkReadiness() error {
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

	if a.CPU < 0 {
		return fmt.Errorf("--cpu %v is negative", a.CPU)
	}

	if a.Memory != "" && !manifest.ValidMemory(a.Memory) {
		return fmt.Errorf("--memory %q is not a valid size: use a byte count or a 1000-based unit (B, KB, MB, GB)",
			a.Memory)
	}

	return nil
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
		Importance: manifest.DefaultImportance,
		Type:       orDefault(a.Type, manifest.TypeService),
		A2AEnabled: a.A2AEnabled,
		Port:       a.Port,
		HealthPath: orDefault(a.HealthPath, manifest.DefaultHealthPath),
		Runtime:    a.runtime(),
		EnvVars:    a.envVars(detected),
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
