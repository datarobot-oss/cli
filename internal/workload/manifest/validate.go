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

package manifest

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"gopkg.in/yaml.v3"
)

// Spec field names the ledger walks. They live here rather than in keys.go
// because they are the ledger's business: every one of them exists because a
// rule below reads it.
const (
	keyAutoscaling        = "autoscaling"
	keyContainerGroups    = "containerGroups"
	keyContainers         = "containers"
	keyDockerfile         = "dockerfile"
	keyEnabled            = "enabled"
	keyEntrypoint         = "entrypoint"
	keyExecEnvID          = "executionEnvironmentId"
	keyExecEnvVersionID   = "executionEnvironmentVersionId"
	keyImageBuildConfig   = "imageBuildConfig"
	keyImageURI           = "imageUri"
	keyMemory             = "memory"
	keyPort               = "port"
	keyPrimary            = "primary"
	keyReplicaCount       = "replicaCount"
	keyResourceAllocation = "resourceAllocation"
	keySpec               = "spec"
)

// Dockerfile sources: whether the user brings the Dockerfile or the platform
// generates one from an execution environment.
const (
	sourceProvided  = "provided"
	sourceGenerated = "generated"
)

// dockerfileName is the only Dockerfile a provided build may name. Setup
// writes no path at all, so this is a fixed expectation of the project root
// rather than a default that can be pointed elsewhere.
const dockerfileName = "Dockerfile"

// minPrimaryPort is the lowest port the primary container may listen on.
// Containers run unprivileged, so a lower port cannot be bound and the
// workload never becomes ready.
const minPrimaryPort = 1024

// memoryPattern accepts a raw byte count or a 1000-based suffix, in any case
// and with optional space, because 512mb and 512 MB are what people type.
// Binary suffixes (Ki, Mi, Gi) are deliberately absent rather than converted:
// the platform reads 4Gi as its decimal namesake, so a file that means 4
// gibibytes would silently get 4 gigabytes.
var memoryPattern = regexp.MustCompile(`^([0-9]+)\s*(B|KB|MB|GB|TB)?$`)

// memoryUnits are the suffixes as the file should spell them.
var memoryUnits = []string{"B", "KB", "MB", "GB", "TB"}

// ValidMemory reports whether value is a size the platform reads the way it
// is written. Exported so a caller can reject a bad --memory at the flag
// rather than at the file it would have produced.
func ValidMemory(value string) bool {
	return memoryPattern.MatchString(strings.ToUpper(strings.TrimSpace(value)))
}

// NormalizeMemory returns value in the spelling the file should carry:
// uppercase unit, no space. It returns value unchanged when it is not a size
// this package recognizes, leaving the verdict to ValidMemory.
func NormalizeMemory(value string) string {
	match := memoryPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(value)))
	if match == nil {
		return value
	}

	return match[1] + match[2]
}

// memoryScale is what each suffix multiplies by. Decimal, not binary: the
// platform reads MB as a million bytes, which is why Mi and friends are
// refused rather than converted.
var memoryScale = map[string]int64{
	"":   1,
	"B":  1,
	"KB": 1_000,
	"MB": 1_000_000,
	"GB": 1_000_000_000,
	"TB": 1_000_000_000_000,
}

// MemoryBytes reads a size into bytes, reporting whether it could.
//
// The platform stores a size as a number of bytes and hands it back that way,
// so "512MB" in the file and 512000000 in the live workload are the same
// sizing written two ways. Anything comparing the two has to do it here or it
// finds a difference on every run, for ever.
func MemoryBytes(value string) (int64, bool) {
	match := memoryPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(value)))
	if match == nil {
		return 0, false
	}

	amount, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, false
	}

	scale, ok := memoryScale[match[2]]
	if !ok {
		return 0, false
	}

	return amount * scale, true
}

// MemoryUnits lists the suffixes a size may carry, for a prompt that would
// rather show them than describe them.
func MemoryUnits() []string {
	return slices.Clone(memoryUnits)
}

// MinPrimaryPort is the lowest port a primary container may listen on.
const MinPrimaryPort = minPrimaryPort

// Validate walks the parse tree and reports every rule the manifest breaks,
// each finding anchored to the line it came from, so a bad file fails in
// milliseconds instead of after a build. Findings are collected in one pass
// and returned as a *ValidationError sorted by line.
//
// The ledger checks what the CLI can check locally and nothing more. Rules
// the platform has not documented are passed through untouched, so platform
// additions need no CLI release and a mistake inside such a block surfaces as
// the server's error. Credential references are checked for syntax here;
// verifying that each one exists is a GET the caller makes with
// Compiled.CredentialRefs.
func (m *Manifest) Validate() error {
	v := &validator{dir: m.Dir}

	v.checkName(m.root)
	v.checkArtifactBinding(m.root)

	groups := v.checkArtifact(mapValue(m.root, keyArtifact))
	v.checkRuntime(mapValue(m.root, keyRuntime), groups)

	if _, errs := collectCredentialRefs(m.root); len(errs) > 0 {
		v.errs = append(v.errs, errs...)
	}

	if len(v.errs) == 0 {
		return nil
	}

	slices.SortStableFunc(v.errs, func(a, b FieldError) int { return cmp.Compare(a.Line, b.Line) })

	return &ValidationError{File: m.fileLabel(), Errors: v.errs}
}

// validator accumulates findings across one pass over the tree.
type validator struct {
	// dir anchors file-existence rules; "" skips them.
	dir  string
	errs []FieldError
}

// add records a finding at node's line. A nil node means the key is missing
// entirely, in which case anchor names the nearest node that does exist so
// the message still points somewhere useful.
func (v *validator) add(node, anchor *yaml.Node, path, format string, args ...any) {
	line := 0

	switch {
	case node != nil:
		line = node.Line
	case anchor != nil:
		line = anchor.Line
	}

	v.errs = append(v.errs, FieldError{Path: path, Line: line, Msg: fmt.Sprintf(format, args...)})
}

// checkName holds the one field the create endpoint always needs.
func (v *validator) checkName(root *yaml.Node) {
	node := mapValue(root, keyName)

	if name, ok := scalarString(node); !ok || name == "" {
		v.add(node, root, keyName, "is required and must be a non-empty string")
	}
}

// checkArtifactBinding holds the create endpoint's either-or: an inline
// artifact to build, or the id of one that already exists.
func (v *validator) checkArtifactBinding(root *yaml.Node) {
	inline := mapValue(root, keyArtifact)
	byID, _ := scalarString(mapValue(root, keyArtifactID))

	if (!isNullish(inline)) == (byID != "") {
		v.add(nil, root, "",
			"exactly one of %s (inline definition) or %s (existing artifact) is required",
			keyArtifact, keyArtifactID)
	}
}

// groupShape is what the runtime block has to match: the artifact's group and
// container names. The two lists are joined by name, so a typo on either side
// means the runtime sizing silently applies to nothing.
type groupShape struct {
	name       string
	containers []string
}

// checkArtifact validates the inline artifact and returns the container-group
// shape the runtime block is checked against. A manifest that binds an
// existing artifact by id returns no shape, and the runtime name checks are
// skipped: the names live server-side where the CLI cannot see them.
func (v *validator) checkArtifact(artifact *yaml.Node) []groupShape {
	if isNullish(artifact) {
		return nil
	}

	spec := mapValue(artifact, keySpec)
	if isNullish(spec) {
		v.add(spec, artifact, joinPath(keyArtifact, keySpec), "is required for an inline artifact")

		return nil
	}

	specPath := joinPath(keyArtifact, keySpec)
	groupsNode := mapValue(spec, keyContainerGroups)
	groups := seqItems(groupsNode)

	if len(groups) == 0 {
		v.add(groupsNode, spec, joinPath(specPath, keyContainerGroups), "must list at least one container group")

		return nil
	}

	shapes := make([]groupShape, 0, len(groups))

	for i, group := range groups {
		shapes = append(shapes, v.checkArtifactGroup(group, fmt.Sprintf("%s.%s[%d]", specPath, keyContainerGroups, i)))
	}

	return shapes
}

func (v *validator) checkArtifactGroup(group *yaml.Node, path string) groupShape {
	shape := groupShape{}

	name, ok := scalarString(mapValue(group, keyName))
	if !ok || name == "" {
		v.add(mapValue(group, keyName), group, joinPath(path, keyName),
			"is required: runtime sizing is joined to the artifact by group name")
	}

	shape.name = name

	containersNode := mapValue(group, keyContainers)
	containers := seqItems(containersNode)

	if len(containers) == 0 {
		v.add(containersNode, group, joinPath(path, keyContainers), "must list at least one container")

		return shape
	}

	primaries := 0

	for i, container := range containers {
		containerPath := fmt.Sprintf("%s.%s[%d]", path, keyContainers, i)

		containerName, ok := scalarString(mapValue(container, keyName))
		if !ok || containerName == "" {
			v.add(mapValue(container, keyName), container, joinPath(containerPath, keyName), "is required")
		}

		shape.containers = append(shape.containers, containerName)

		primary := isTrue(mapValue(container, keyPrimary))
		if primary {
			primaries++
		}

		v.checkImageSource(container, containerPath)
		v.checkPort(container, containerPath, primary)
	}

	// Two primaries in one group is unambiguously wrong: only one container
	// can be the one traffic reaches. Zero is left to the server, which knows
	// whether an auxiliary group without an entry point is legal.
	if primaries > 1 {
		v.add(group, group, path, "has %d containers marked %s; only one can be", primaries, keyPrimary)
	}

	return shape
}

// checkImageSource holds the exactly-one rule: a container names an image or
// says how to build one, never both and never neither.
func (v *validator) checkImageSource(container *yaml.Node, path string) {
	imageURI, hasImage := scalarString(mapValue(container, keyImageURI))
	hasImage = hasImage && imageURI != ""
	buildConfig := mapValue(container, keyImageBuildConfig)
	hasBuildConfig := !isNullish(buildConfig)

	switch {
	case hasImage && hasBuildConfig:
		v.add(buildConfig, container, path,
			"sets both %s and %s; a container names an image or says how to build one, never both",
			keyImageURI, keyImageBuildConfig)
	case !hasImage && !hasBuildConfig:
		v.add(nil, container, path, "must set either %s or %s", keyImageURI, keyImageBuildConfig)
	case hasBuildConfig:
		v.checkBuildConfig(buildConfig, joinPath(path, keyImageBuildConfig))
	}
}

// checkBuildConfig validates a build block. A misspelled key inside it lands
// here rather than passing through: without a readable dockerfile block there
// is no build to run, so the file is wrong in a way the platform cannot
// reinterpret later.
func (v *validator) checkBuildConfig(buildConfig *yaml.Node, path string) {
	dockerfile := mapValue(buildConfig, keyDockerfile)
	if isNullish(dockerfile) {
		v.add(nil, buildConfig, joinPath(path, keyDockerfile),
			"is required: %s must say how the image is built", keyImageBuildConfig)

		return
	}

	dockerfilePath := joinPath(path, keyDockerfile)
	sourceNode := mapValue(dockerfile, keySource)
	source, _ := scalarString(sourceNode)

	switch source {
	case sourceProvided:
		v.checkProvidedBuild(dockerfile, dockerfilePath)
	case sourceGenerated:
		v.checkGeneratedBuild(dockerfile, dockerfilePath)
	default:
		v.add(sourceNode, dockerfile, joinPath(dockerfilePath, keySource),
			"must be %q (build the repository's %s) or %q (build from a DataRobot base image)",
			sourceProvided, dockerfileName, sourceGenerated)
	}
}

// checkProvidedBuild fails a provided build with no Dockerfile to build,
// which would otherwise be discovered only once the server-side build starts.
func (v *validator) checkProvidedBuild(dockerfile *yaml.Node, path string) {
	if v.dir == "" {
		return
	}

	if !fsutil.FileExists(filepath.Join(v.dir, dockerfileName)) {
		v.add(dockerfile, dockerfile, path,
			"source is %q but no %s exists in %s", sourceProvided, dockerfileName, v.dir)
	}
}

// checkGeneratedBuild holds the three fields a generated build cannot be
// reproduced without. Both environment ids are recorded, not just the name,
// so the same file builds the same image after the environment moves on.
func (v *validator) checkGeneratedBuild(dockerfile *yaml.Node, path string) {
	for _, key := range []string{keyExecEnvID, keyExecEnvVersionID} {
		node := mapValue(dockerfile, key)

		if value, ok := scalarString(node); !ok || value == "" {
			v.add(node, dockerfile, joinPath(path, key),
				"is required when source is %q", sourceGenerated)
		}
	}

	entrypoint := mapValue(dockerfile, keyEntrypoint)
	if len(seqItems(entrypoint)) == 0 {
		v.add(entrypoint, dockerfile, joinPath(path, keyEntrypoint),
			"is required when source is %q and must list at least one argument", sourceGenerated)
	}
}

// checkPort holds the port rules that decide whether traffic can reach the
// container at all.
func (v *validator) checkPort(container *yaml.Node, path string, primary bool) {
	node := mapValue(container, keyPort)
	portPath := joinPath(path, keyPort)

	if !primary {
		if node != nil {
			v.add(node, container, portPath,
				"is only allowed on the primary container; traffic reaches the group through that one")
		}

		return
	}

	port, ok := scalarInt(node)
	if !ok {
		v.add(node, container, portPath, "is required on the primary container and must be a number")

		return
	}

	if port < minPrimaryPort {
		v.add(node, container, portPath,
			"is %d; the primary container runs unprivileged and must listen on %d or above", port, minPrimaryPort)
	}
}

// checkRuntime validates the sizing half of the file against the shape of the
// artifact half.
func (v *validator) checkRuntime(runtime *yaml.Node, shapes []groupShape) {
	if runtime == nil {
		return
	}

	for i, group := range seqItems(mapValue(runtime, keyContainerGroups)) {
		path := fmt.Sprintf("%s.%s[%d]", keyRuntime, keyContainerGroups, i)

		v.checkScaling(group, path)

		name, _ := scalarString(mapValue(group, keyName))
		shape, matched := findShape(shapes, name)

		if shapes != nil && !matched {
			v.add(mapValue(group, keyName), group, joinPath(path, keyName),
				"%q matches no artifact container group (have %s)", name, quotedNames(shapeNames(shapes)))
		}

		v.checkRuntimeContainers(group, path, shape, matched)
	}
}

func (v *validator) checkRuntimeContainers(group *yaml.Node, path string, shape groupShape, matched bool) {
	for i, container := range seqItems(mapValue(group, keyContainers)) {
		containerPath := fmt.Sprintf("%s.%s[%d]", path, keyContainers, i)
		name, _ := scalarString(mapValue(container, keyName))

		if matched && !slices.Contains(shape.containers, name) {
			v.add(mapValue(container, keyName), container, joinPath(containerPath, keyName),
				"%q matches no container in artifact group %q (have %s)",
				name, shape.name, quotedNames(shape.containers))
		}

		v.checkMemory(mapValue(container, keyResourceAllocation), joinPath(containerPath, keyResourceAllocation))
	}
}

// checkScaling holds replicaCount against autoscaling. A group may say how
// many replicas to run or how to scale them, not both. Autoscaling switched
// off is not scaling, so an explicit enabled:false leaves replicaCount alone,
// matching what the create endpoint accepts. A null or empty autoscaling block
// is also not a policy, matching the server's own echo.
func (v *validator) checkScaling(group *yaml.Node, path string) {
	replicas := mapValue(group, keyReplicaCount)
	autoscaling := mapValue(group, keyAutoscaling)

	if replicas == nil || isNullish(autoscaling) {
		return
	}

	enabled := mapValue(autoscaling, keyEnabled)
	if enabled != nil && !isTrue(enabled) {
		return
	}

	v.add(replicas, group, joinPath(path, keyReplicaCount),
		"cannot be set alongside %s; omit one of them", keyAutoscaling)
}

// checkMemory holds the unit rule for a container's memory allocation.
func (v *validator) checkMemory(allocation *yaml.Node, path string) {
	node := mapValue(allocation, keyMemory)

	value, ok := scalarString(node)
	if !ok {
		return
	}

	if !ValidMemory(value) {
		v.add(node, allocation, joinPath(path, keyMemory),
			"%q is not a valid size: use a byte count or a 1000-based unit (B, KB, MB, GB). "+
				"Binary units such as Gi are read as their decimal namesakes, so they are rejected rather than silently converted",
			value)
	}
}

func findShape(shapes []groupShape, name string) (groupShape, bool) {
	for _, shape := range shapes {
		if shape.name == name {
			return shape, true
		}
	}

	return groupShape{}, false
}

func shapeNames(shapes []groupShape) []string {
	names := make([]string, 0, len(shapes))
	for _, shape := range shapes {
		names = append(names, shape.name)
	}

	return names
}

// quotedNames renders a name list for an error message, so the fix is visible
// without opening the other half of the file.
func quotedNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}

	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}

	return strings.Join(quoted, ", ")
}
