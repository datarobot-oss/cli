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

// Package wlconfig reads and writes .datarobot/workload.yaml, the committed
// manifest that `dr workload config` generates and `dr workload up` consumes to
// build a container image and deploy it as a workload. It intentionally mirrors
// the "Git to Workload" manifest shape (name/importance/build/runtime).
package wlconfig

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"gopkg.in/yaml.v3"
)

const (
	// DirName is the DataRobot project directory that holds the committed
	// workload manifest.
	DirName = ".datarobot"

	// FileName is the committed workload manifest inside DirName.
	FileName = "workload.yaml"
)

// Build modes, discriminated by which fields are set.
const (
	// ModeProvided builds the user's own Dockerfile server-side.
	ModeProvided = "provided"
	// ModeGenerated builds from an execution environment (no Dockerfile).
	ModeGenerated = "generated"
	// ModeImage deploys an existing container image; no code, no build.
	ModeImage = "image"
)

// Defaults used by Default() and the generated manifest.
const (
	DefaultImportance = "low"
	DefaultDockerfile = "./Dockerfile"
	DefaultPort       = 8080
	DefaultHealth     = "/health"
	DefaultReplicas   = 1
	DefaultCPU        = 0.5
	DefaultMemory     = "512MB"
	// DefaultExecutionEnvironment is the EE name `up` resolves for generated
	// mode when the manifest names none.
	DefaultExecutionEnvironment = "[DataRobot] Python 3.12 Applications Base"
)

// ErrNotConfigured is returned by Load when no workload.yaml exists yet, so
// callers (e.g. `dr workload up`) can point the user at `dr workload config`.
var ErrNotConfigured = errors.New("workload not configured: .datarobot/workload.yaml not found")

// Config is the parsed workload manifest.
type Config struct {
	WorkloadID string   `yaml:"workloadId,omitempty"`
	Name       string   `yaml:"name,omitempty"`
	Importance string   `yaml:"importance,omitempty"`
	Build      *Build   `yaml:"build,omitempty"`
	Runtime    *Runtime `yaml:"runtime,omitempty"`
	// Env is injected into the container as environment variables. Values may
	// reference the deploying shell's environment as ${VAR}; `up` expands them
	// at deploy time so secrets never live in this committed file.
	Env map[string]string `yaml:"env,omitempty"`
}

// Build describes how the container image is produced. Exactly one mode is
// active: a non-empty Image deploys that pre-built image (no build); otherwise
// a non-empty Dockerfile selects provided mode; otherwise `up` uses generated
// mode with ExecutionEnvironment + Entrypoint.
type Build struct {
	Image                string   `yaml:"image,omitempty"`
	Dockerfile           string   `yaml:"dockerfile,omitempty"`
	ExecutionEnvironment string   `yaml:"executionEnvironment,omitempty"`
	Entrypoint           []string `yaml:"entrypoint,omitempty"`
	Port                 int      `yaml:"port,omitempty"`
	Health               string   `yaml:"health,omitempty"`
}

// Runtime describes the deployed workload's resources.
type Runtime struct {
	Replicas int     `yaml:"replicas,omitempty"`
	CPU      float64 `yaml:"cpu,omitempty"`
	Memory   string  `yaml:"memory,omitempty"`
}

// Default returns a fully-populated provided-Dockerfile manifest for name, so
// `dr workload up` can build and deploy with no further input.
func Default(name string) Config {
	return Config{
		Name:       name,
		Importance: DefaultImportance,
		Build:      &Build{Dockerfile: DefaultDockerfile, Port: DefaultPort, Health: DefaultHealth},
		Runtime:    &Runtime{Replicas: DefaultReplicas, CPU: DefaultCPU, Memory: DefaultMemory},
	}
}

// BuildMode reports which build mode the manifest selects. Image wins over
// Dockerfile so a user switching modes only has to add the image line.
func (c Config) BuildMode() string {
	switch {
	case c.Build != nil && c.Build.Image != "":
		return ModeImage
	case c.Build != nil && c.Build.Dockerfile != "":
		return ModeProvided
	default:
		return ModeGenerated
	}
}

// Port returns the configured container port or the default.
func (c Config) Port() int {
	if c.Build != nil && c.Build.Port > 0 {
		return c.Build.Port
	}

	return DefaultPort
}

// Health returns the configured probe path or the default.
func (c Config) Health() string {
	if c.Build != nil && c.Build.Health != "" {
		return c.Build.Health
	}

	return DefaultHealth
}

// Dir returns the .datarobot directory for projectDir.
func Dir(projectDir string) string {
	return filepath.Join(projectDir, DirName)
}

// Path returns the absolute path to the manifest for projectDir.
func Path(projectDir string) string {
	return filepath.Join(Dir(projectDir), FileName)
}

// Exists reports whether projectDir already has a committed manifest.
func Exists(projectDir string) bool {
	return fsutil.FileExists(Path(projectDir))
}

// Load reads and parses the manifest, returning ErrNotConfigured when absent.
func Load(projectDir string) (Config, error) {
	path := Path(projectDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, ErrNotConfigured
		}

		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}

// Save atomically writes cfg to the manifest as a commented, self-documenting
// file (regenerated each write, so `up`'s workloadId write-back keeps the
// comments intact).
func Save(projectDir string, cfg Config) error {
	dir := Dir(projectDir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	return atomicWriteFile(Path(projectDir), Marshal(cfg))
}

// Marshal renders cfg as the commented manifest. Fields fall back to defaults so
// a partial Config still produces a runnable file.
func Marshal(cfg Config) []byte {
	b := cfg.withDefaults()

	var sb strings.Builder

	sb.WriteString("# DataRobot workload manifest, generated by `dr workload config`.\n")
	sb.WriteString("# `dr workload up` reads this to deploy a workload: it builds a container\n")
	sb.WriteString("# image from this repo (or takes your pre-built one) and creates everything\n")
	sb.WriteString("# else automatically.\n\n")

	if b.WorkloadID != "" {
		fmt.Fprintf(&sb, "workloadId: %s\n", b.WorkloadID)
	}

	fmt.Fprintf(&sb, "name: %s\n", b.Name)
	fmt.Fprintf(&sb, "importance: %s   # low | moderate | high | critical\n\n", b.Importance)

	sb.WriteString("build:\n")
	writeBuildSection(&sb, b)

	sb.WriteString("\nruntime:\n")
	fmt.Fprintf(&sb, "  replicas: %d\n", b.Runtime.Replicas)
	fmt.Fprintf(&sb, "  cpu: %s        # cores (fractional allowed)\n", formatFloat(b.Runtime.CPU))
	fmt.Fprintf(&sb, "  memory: %s   # B/KB/MB/GB\n", b.Runtime.Memory)

	writeEnvSection(&sb, b)

	return []byte(sb.String())
}

// writeBuildSection renders the active build mode, with the other modes' fields
// shown as commented hints.
func writeBuildSection(sb *strings.Builder, c Config) {
	switch c.BuildMode() {
	case ModeImage:
		sb.WriteString("  # Pre-built image mode: deploy an existing container image, no build step.\n")
		fmt.Fprintf(sb, "  image: %s\n", c.Build.Image)

		if len(c.Build.Entrypoint) > 0 {
			fmt.Fprintf(sb, "  entrypoint: [%s]\n", quoteList(c.Build.Entrypoint))
		}

		sb.WriteString("  # Or build from this repo instead: remove `image` and set either\n")
		sb.WriteString("  # `dockerfile: ./Dockerfile` or `executionEnvironment` + `entrypoint`.\n")
	case ModeProvided:
		sb.WriteString("  # Provided-Dockerfile mode: DataRobot builds your Dockerfile from this repo.\n")
		fmt.Fprintf(sb, "  dockerfile: %s\n", c.Build.Dockerfile)
		sb.WriteString("  # Other modes: replace `dockerfile` with one of:\n")
		fmt.Fprintf(sb, "  #   executionEnvironment: %q   # + entrypoint: [...]\n", DefaultExecutionEnvironment)
		sb.WriteString("  #   image: registry/repo:tag                              # deploy a pre-built image\n")
	default:
		sb.WriteString("  # Generated mode: DataRobot builds an image from an execution environment.\n")
		fmt.Fprintf(sb, "  executionEnvironment: %q\n", c.Build.ExecutionEnvironment)
		fmt.Fprintf(sb, "  entrypoint: [%s]\n", quoteList(c.Build.Entrypoint))
		sb.WriteString("  # Other modes: replace the two lines above with one of:\n")
		sb.WriteString("  #   dockerfile: ./Dockerfile     # build your own Dockerfile\n")
		sb.WriteString("  #   image: registry/repo:tag     # deploy a pre-built image\n")
	}

	fmt.Fprintf(sb, "  port: %d          # container listen port (>= 1024)\n", c.Build.Port)
	fmt.Fprintf(sb, "  health: %s      # HTTP readiness/liveness path\n", c.Build.Health)
}

// writeEnvSection renders the container environment variables, or a commented
// example when none are set.
func writeEnvSection(sb *strings.Builder, c Config) {
	sb.WriteString("\n# Container environment variables; ${VAR} expands from your shell at deploy\n")
	sb.WriteString("# time, so secrets never live in this committed file.\n")

	if len(c.Env) == 0 {
		sb.WriteString("# env:\n")
		sb.WriteString("#   MY_SETTING: value\n")
		sb.WriteString("#   MY_SECRET: ${MY_SECRET}\n")

		return
	}

	sb.WriteString("env:\n")

	for _, k := range slices.Sorted(maps.Keys(c.Env)) {
		fmt.Fprintf(sb, "  %s: %q\n", k, c.Env[k])
	}
}

// withDefaults returns a copy of cfg with empty fields filled from Default, so
// Marshal never emits a blank required field.
func (c Config) withDefaults() Config {
	out := c

	if out.Name == "" {
		out.Name = "my-app"
	}

	if out.Importance == "" {
		out.Importance = DefaultImportance
	}

	out.Build = buildWithDefaults(out.Build)
	out.Runtime = runtimeWithDefaults(out.Runtime)

	return out
}

func buildWithDefaults(b *Build) *Build {
	out := Build{}
	if b != nil {
		out = *b
	}

	if out.Image == "" && out.Dockerfile == "" && out.ExecutionEnvironment == "" {
		out.Dockerfile = DefaultDockerfile
	}

	if out.Port == 0 {
		out.Port = DefaultPort
	}

	if out.Health == "" {
		out.Health = DefaultHealth
	}

	return &out
}

func runtimeWithDefaults(r *Runtime) *Runtime {
	out := Runtime{}
	if r != nil {
		out = *r
	}

	if out.Replicas == 0 {
		out.Replicas = DefaultReplicas
	}

	if out.CPU == 0 {
		out.CPU = DefaultCPU
	}

	if out.Memory == "" {
		out.Memory = DefaultMemory
	}

	return &out
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))

	for i, s := range items {
		quoted[i] = strconv.Quote(s)
	}

	return strings.Join(quoted, ", ")
}

// atomicWriteFile writes data via a temp file + rename so readers never observe
// a half-written manifest.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".workload-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}

	tmpName := tmp.Name()

	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write %s: %w", tmpName, err)
	}

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}

	return nil
}
