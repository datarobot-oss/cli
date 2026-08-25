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
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// serverManagedKeys never belong in a manifest, at any depth. They are the
// server's own bookkeeping: it assigns them, and a file that repeats them
// back is either ignored or rejected. Stripping them recursively is safe
// because the create spec has no legitimate key by these names anywhere.
var serverManagedKeys = []string{
	"id", "createdAt", "updatedAt", "status", "endpoint", "artifactId", "workloadId", "resolvedBundle",
	"imageOutdated",
}

// Live is a running workload, downloaded so a manifest can be written that
// says everything the workload already says. Setup uses it when the user
// binds to an existing workload: the alternative, generating a file from the
// wizard's own small vocabulary, would silently drop the autoscaling, the
// extra containers and the GPU bundle that workload is running with, and the
// next deploy would apply that loss.
//
// Spec and Runtime are held as the server's own documents rather than parsed
// into structs, for the same reason the reader is node-based: a block this
// release has never heard of still has to survive the round trip.
type Live struct {
	WorkloadID   string
	Name         string
	Importance   string
	ArtifactName string
	// ArtifactType is the artifact's kind, which the platform keeps beside the
	// spec rather than inside it.
	ArtifactType string
	Spec         map[string]any
	Runtime      map[string]any
}

// NewLive assembles the live view from the two documents the server returns,
// stripping what only the server may set. workloadDoc is the workload,
// artifactDoc the artifact it currently runs.
//
// It fails loudly when the artifact doc carries no usable spec — absent, or
// stripped to nothing by the server-managed-key purge — so a partial or
// permission-filtered artifact GET can never produce a manifest that Validate
// would reject with "artifact.spec: is required for an inline artifact". The
// error names the workload id, mirroring Apply's own "no primary container"
// contract, because the right answer is to edit the file by hand rather than
// let the CLI write something it would then refuse to read.
func NewLive(workloadID string, workloadDoc, artifactDoc map[string]any) (Live, error) {
	live := Live{
		WorkloadID:   workloadID,
		Name:         stringAt(workloadDoc, keyName),
		Importance:   stringAt(workloadDoc, keyImportance),
		ArtifactName: stringAt(artifactDoc, keyName),
		ArtifactType: stringAt(artifactDoc, keyType),
		// stripServerManaged deep-copies once and strips both server-managed
		// keys and nil-valued keys in a single recursive walk, so no second
		// copy is allocated here.
		Spec:    stripServerManaged(mapAt(artifactDoc, keySpec)),
		Runtime: stripServerManaged(mapAt(workloadDoc, keyRuntime)),
	}

	stripBuildOutputs(live.Spec)

	if len(live.Spec) == 0 {
		return live, fmt.Errorf("workload %s has no artifact spec to bind; edit %s by hand instead",
			workloadID, FileName)
	}

	return live, nil
}

// Type reports the artifact's kind, so the wizard's Q3 opens on what the
// workload already is.
func (l Live) Type() string {
	return l.ArtifactType
}

// Defaults reads the live spec into the answers the wizard's screens open on.
// Everything it cannot find keeps the documented default, so a spec shaped in
// a way this release does not recognize still produces usable screens; the
// parts it did not understand are preserved in Spec regardless.
func (l Live) Defaults() Draft {
	draft := Draft{
		WorkloadID: l.WorkloadID,
		Name:       l.Name,
		Importance: orDefaultString(l.Importance, DefaultImportance),
		Type:       ArtifactTypeOrDefault(l.Type()),
		A2AEnabled: boolAt(l.Spec, keyA2AEnabled),
		Port:       DefaultPort,
		Build:      Build{Mode: BuildModeDockerfile},
		Runtime:    l.runtimeDefaults(),
	}

	container := l.primaryContainer()
	if container == nil {
		return draft
	}

	if port, ok := intAt(container, keyPort); ok {
		draft.Port = port
	}

	// The health path is left empty when the container has no readiness probe,
	// rather than falling back to DefaultHealthPath like everything else here.
	// A workload running without a probe has to arrive saying so: the screen
	// shows the absence, and Apply can then read a path as the answer someone
	// gave rather than as a default it must be careful not to attach.
	if path := stringAt(mapAt(container, keyReadinessProbe), keyPath); path != "" {
		draft.HealthPath = path
	}

	draft.Build = liveBuild(container)

	return draft
}

// ReadinessProbe reports what the running workload does about readiness:
// whether the primary container declares a probe at all, and whether this
// release can read a path out of it.
//
// The two are separate questions, and Defaults answers neither on its own: it
// reports a probe with no readable path the same way it reports no probe, as an
// empty HealthPath. Only the screens care about the difference, and they care a
// lot, because an empty field means "no probe" to everything downstream.
func (l Live) ReadinessProbe() (present, readable bool) {
	container := l.primaryContainer()
	if container == nil {
		return false, false
	}

	probe := mapAt(container, keyReadinessProbe)
	if probe == nil {
		return false, false
	}

	return true, stringAt(probe, keyPath) != ""
}

// BuildMode reports how the primary container's image comes to be, in the
// wizard's vocabulary: BuildModeDockerfile, BuildModeGenerated or
// BuildModeImage. It is "" for a manifest with no inline artifact, which is
// what a manifest bound to an existing artifact by id looks like.
func (m *Manifest) BuildMode() string {
	container := primaryContainerNode(mapValue(mapValue(m.root, keyArtifact), keySpec))
	if container == nil {
		return ""
	}

	dockerfile := mapValue(mapValue(container, keyImageBuildConfig), keyDockerfile)
	source, _ := scalarString(mapValue(dockerfile, keySource))

	switch source {
	case sourceProvided:
		return BuildModeDockerfile
	case sourceGenerated:
		return BuildModeGenerated
	}

	if uri, _ := scalarString(mapValue(container, keyImageURI)); uri != "" {
		return BuildModeImage
	}

	return ""
}

// primaryContainerNode finds the container traffic reaches in a parsed spec,
// falling back to the first container the way the live-document walk does.
func primaryContainerNode(spec *yaml.Node) *yaml.Node {
	var first *yaml.Node

	for _, group := range seqItems(mapValue(spec, keyContainerGroups)) {
		for _, container := range seqItems(mapValue(group, keyContainers)) {
			if first == nil {
				first = container
			}

			if isTrue(mapValue(container, keyPrimary)) {
				return container
			}
		}
	}

	return first
}

// runtimeDefaults reads the sizing the workload is actually running with, so
// the confirm screen reports that rather than the documented defaults. A
// group using autoscaling has no replica count to report, and the zero it
// leaves behind is what tells Apply not to write one.
func (l Live) runtimeDefaults() Runtime {
	runtime := Runtime{Replicas: DefaultReplicas, CPU: DefaultCPU, Memory: DefaultMemory}

	groups := slicesAt(l.Runtime, keyContainerGroups)
	if len(groups) == 0 {
		return runtime
	}

	group := groups[0]

	if replicas, ok := intAt(group, keyReplicaCount); ok {
		runtime.Replicas = replicas
	}

	if autoscalingActive(group) {
		runtime.Replicas = 0
	}

	// The container Apply writes to, not whichever the server listed first. A
	// group can list a sidecar ahead of the primary, and seeding the wizard
	// from that would have the user accept the sidecar's sizing onto the
	// primary and silently shrink what is running. A group that does not list
	// the primary at all has no sizing to report, and Apply creates the entry.
	container := findRuntimeContainer(group, l.primaryContainerName())
	if container == nil {
		return runtime
	}

	allocation := mapAt(container, keyResourceAllocation)

	if cpu, ok := floatAt(allocation, keyCPU); ok {
		runtime.CPU = cpu
	}

	if memory := stringAt(allocation, keyMemory); memory != "" {
		runtime.Memory = memory
	}

	return runtime
}

// liveBuild reads which image source the container uses. A build block whose
// source this release does not recognize reports BuildModeUnchanged rather
// than being forced into the nearest known mode: the block is preserved as it
// stands, and the wizard offers to keep it.
func liveBuild(container map[string]any) Build {
	buildConfig := mapAt(container, keyImageBuildConfig)
	dockerfile := mapAt(buildConfig, keyDockerfile)

	switch stringAt(dockerfile, keySource) {
	case sourceProvided:
		return Build{Mode: BuildModeDockerfile}
	case sourceGenerated:
		return Build{
			Mode:                          BuildModeGenerated,
			ExecutionEnvironmentID:        stringAt(dockerfile, keyExecEnvID),
			ExecutionEnvironmentVersionID: stringAt(dockerfile, keyExecEnvVersionID),
			Entrypoint:                    stringsAt(dockerfile, keyEntrypoint),
		}
	}

	if len(buildConfig) > 0 {
		return Build{Mode: BuildModeUnchanged}
	}

	return Build{Mode: BuildModeImage, ImageURI: stringAt(container, keyImageURI)}
}

// Apply writes the wizard's answers onto the live spec and returns the
// result, leaving the original untouched so the confirm screen can diff one
// against the other. Only the fields the wizard actually asks about are
// touched; everything else the workload runs with is carried over as it is.
func (l Live) Apply(draft Draft) (Live, error) {
	applied := l
	applied.Name = draft.Name
	applied.Importance = draft.Importance
	applied.Spec = deepCopyMap(l.Spec)
	applied.Runtime = deepCopyMap(l.Runtime)

	if applied.Spec == nil {
		applied.Spec = map[string]any{}
	}

	applied.ArtifactType = draft.Type

	// Any type the server echoed inside the spec is noise: it pops spec.type
	// on the way in and reads the artifact's own type instead.
	delete(applied.Spec, keyType)

	if draft.Type == TypeAgent && draft.A2AEnabled {
		applied.Spec[keyA2AEnabled] = true
	} else {
		delete(applied.Spec, keyA2AEnabled)
	}

	container := primaryContainerOf(applied.Spec)
	if container == nil {
		return Live{}, fmt.Errorf("workload %s has no primary container to update; edit %s by hand instead",
			l.WorkloadID, FileName)
	}

	applyReadiness(container, draft)
	applyEnvVars(container, draft.EnvVars)
	applied.applyRuntime(draft.Runtime)

	if err := applyBuild(container, draft.Build); err != nil {
		return Live{}, err
	}

	return applied, nil
}

// applyEnvVars merges the .env variables into the container's existing list.
// A name the workload already declares is left alone: the running value is
// the one in use, and .env is the developer's copy, which may well be stale.
func applyEnvVars(container map[string]any, vars []EnvVar) {
	if len(vars) == 0 {
		return
	}

	existing, _ := container[keyEnvironmentVars].([]any)
	declared := declaredEnvNames(existing)

	for _, v := range vars {
		if declared[v.Name] {
			continue
		}

		value := v.Value
		if v.Secret {
			value = CredentialShorthandPrefix + v.credentialID() + "/" + credentialKey
		}

		existing = append(existing, map[string]any{keyName: v.Name, keyValue: value})
	}

	container[keyEnvironmentVars] = existing
}

// NewEnvVars is the subset of vars that Apply would actually add: the names
// the workload does not already declare. A caller reporting what a bind did
// has to count these rather than everything it classified, or it claims to
// have added variables that kept their running values.
func (l Live) NewEnvVars(vars []EnvVar) []EnvVar {
	container := primaryContainerOf(l.Spec)
	if container == nil {
		return vars
	}

	existing, _ := container[keyEnvironmentVars].([]any)

	declared := declaredEnvNames(existing)
	fresh := make([]EnvVar, 0, len(vars))

	for _, v := range vars {
		if !declared[v.Name] {
			fresh = append(fresh, v)
		}
	}

	return fresh
}

// LiteralEnvVars is the variables the workload already declares with the value
// written out, rather than as a reference to a credential.
//
// Binding preserves the live spec, which is what stops it downgrading a
// running workload, and that applies to these entries too: a value the
// platform holds in the clear is copied into a file meant to be committed.
// Nothing here can be fixed automatically, because only the platform can say
// whether a given value is sensitive. A caller that wants to raise it needs
// the values to judge them by, so they come out with the names.
func (l Live) LiteralEnvVars() []EnvVar {
	container := primaryContainerOf(l.Spec)
	if container == nil {
		return nil
	}

	existing, _ := container[keyEnvironmentVars].([]any)
	literals := make([]EnvVar, 0, len(existing))

	for _, entry := range existing {
		typed, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		// A credential-backed entry has no value to read: the object form
		// names an id and a key instead, and the shorthand carries them in a
		// string that is a reference rather than the secret it points at.
		value := stringAt(typed, keyValue)
		if value == "" || strings.HasPrefix(value, CredentialShorthandPrefix) {
			continue
		}

		literals = append(literals, EnvVar{Name: stringAt(typed, keyName), Value: value})
	}

	return literals
}

// declaredEnvNames is the set of variable names an environmentVars list
// already carries.
func declaredEnvNames(existing []any) map[string]bool {
	declared := make(map[string]bool, len(existing))

	for _, entry := range existing {
		if typed, ok := entry.(map[string]any); ok {
			declared[stringAt(typed, keyName)] = true
		}
	}

	return declared
}

// applyRuntime writes the sizing answers onto the live runtime block, leaving
// alone what the wizard has no question for: the GPU bundles, the autoscaling
// policies and every container past the first. A zero field means "not
// answered", so it keeps whatever the workload is running with.
func (l *Live) applyRuntime(runtime Runtime) {
	group := l.runtimeGroup()
	if group == nil {
		return
	}

	// Writing a replica count onto an autoscaled group would produce a file
	// the ledger rejects, so an autoscaled workload keeps its autoscaling and
	// the answer is dropped.
	if runtime.Replicas > 0 && !autoscalingActive(group) {
		group[keyReplicaCount] = runtime.Replicas
	}

	container := runtimeContainer(group, l.primaryContainerName())
	if container == nil {
		return
	}

	allocation := mapAt(container, keyResourceAllocation)
	if allocation == nil {
		allocation = map[string]any{}
		container[keyResourceAllocation] = allocation
	}

	if runtime.CPU > 0 {
		allocation[keyCPU] = runtime.CPU
	}

	if runtime.Memory != "" {
		allocation[keyMemory] = runtime.Memory
	}
}

// runtimeGroup is the sizing block the answers apply to, created to match the
// artifact when the workload has none. A workload can legitimately run
// without a runtime block, and returning early there would drop every sizing
// answer the user just gave without saying so.
func (l *Live) runtimeGroup() map[string]any {
	if groups := slicesAt(l.Runtime, keyContainerGroups); len(groups) > 0 {
		return groups[0]
	}

	name := l.artifactGroupName()
	if name == "" {
		return nil
	}

	group := map[string]any{keyName: name, keyContainers: []any{}}

	if l.Runtime == nil {
		l.Runtime = map[string]any{}
	}

	l.Runtime[keyContainerGroups] = []any{group}

	return group
}

// autoscalingActive reports whether the group is actually autoscaling. A block
// that is present but switched off is not, which is the rule the create-payload
// check and the manifest ledger already apply. Live agreeing with them is what
// keeps a replica answer from being dropped for a workload that is not
// autoscaling: the file it would produce is one the ledger accepts.
func autoscalingActive(group map[string]any) bool {
	autoscaling := mapAt(group, keyAutoscaling)
	if len(autoscaling) == 0 {
		return false
	}

	// Absent means on, matching the platform's own default.
	enabled, present := autoscaling[keyEnabled]
	if !present || enabled == nil {
		return true
	}

	on, ok := enabled.(bool)

	return ok && on
}

// runtimeContainer is the sizing entry for name, created when the group does
// not list it yet. The runtime and artifact lists are joined by name, so a
// container invented here has to carry the artifact's.
func runtimeContainer(group map[string]any, name string) map[string]any {
	if name == "" {
		return nil
	}

	if container := findRuntimeContainer(group, name); container != nil {
		return container
	}

	container := map[string]any{keyName: name}
	existing, _ := group[keyContainers].([]any)
	group[keyContainers] = append(existing, container)

	return container
}

// findRuntimeContainer is the sizing entry for name, nil when the group does
// not list one. Kept separate from runtimeContainer so the read path can ask
// the question without the write path's side effect of inventing an entry.
func findRuntimeContainer(group map[string]any, name string) map[string]any {
	if name == "" {
		return nil
	}

	for _, container := range slicesAt(group, keyContainers) {
		if stringAt(container, keyName) == name {
			return container
		}
	}

	return nil
}

// artifactGroupName and primaryContainerName name the artifact's own group
// and primary container, which the runtime block has to match.
func (l Live) artifactGroupName() string {
	groups := slicesAt(l.Spec, keyContainerGroups)
	if len(groups) == 0 {
		return ""
	}

	return stringAt(groups[0], keyName)
}

func (l Live) primaryContainerName() string {
	container := l.primaryContainer()
	if container == nil {
		return ""
	}

	return stringAt(container, keyName)
}

// applyReadiness writes the port and points the readiness probe wherever the
// answers say, which includes having no probe at all.
//
// The draft's path is empty unless something asked for one: Defaults leaves it
// empty for a container running without a probe, so a workload deliberately
// running without one cannot acquire the wizard's default /health, which a
// container that does not serve it would never pass. What reaches here
// non-empty was typed on a screen or passed as a flag, and adding the probe is
// then the answer rather than an invention.
func applyReadiness(container map[string]any, draft Draft) {
	previous, hadPort := intAt(container, keyPort)

	container[keyPort] = draft.Port

	// The container being edited is the one traffic reaches, and the ledger
	// only allows a port on a container that says so. A spec that flagged
	// nothing would otherwise produce a file rejecting its own port.
	container[keyPrimary] = true

	rewritten := applyProbe(container, draft)

	if !hadPort || previous == draft.Port {
		return
	}

	// Probes the wizard has no question for are preserved as they stand. One
	// whose own port was watching the container's still has to follow it: a
	// startup probe left behind on the old port is how a changed port becomes a
	// deploy that never reports ready. A probe aimed somewhere else was aimed
	// there deliberately and keeps its own port.
	//
	// Nested ports follow too. A tcpSocket or gRPC probe keeps its port one
	// level down, and leaving that on a port the container no longer listens on
	// is the same never-reports-ready deploy this loop exists to prevent. Only
	// a port that matched the container's old one moves, so a probe deliberately
	// aimed elsewhere is still left alone whatever shape it is written in.
	following := []string{keyStartupProbe, keyLivenessProbe}
	if !rewritten {
		// A readiness probe this release could not read was left alone above,
		// so it follows the same rule as the other two rather than keeping a
		// port the container no longer listens on.
		following = append(following, keyReadinessProbe)
	}

	for _, key := range following {
		followPort(mapAt(container, key), previous, draft.Port)
	}
}

// followPort moves a probe from one port to another, at the probe's own level
// and one level down, which is where the shapes this release does not model
// keep theirs. It descends no further: past that, a "port" is as likely to
// belong to something the probe merely mentions as to the thing it dials.
func followPort(probe map[string]any, from, to int) {
	if probe == nil {
		return
	}

	if port, ok := intAt(probe, keyPort); ok && port == from {
		probe[keyPort] = to
	}

	for _, value := range probe {
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}

		if port, ok := intAt(nested, keyPort); ok && port == from {
			nested[keyPort] = to
		}
	}
}

// applyProbe settles the readiness probe and reports whether it rewrote or
// removed one, so the caller knows whether the block still needs the port
// treatment every other probe gets.
//
// A block whose path this release cannot read is left exactly as it stands. An
// empty draft path means "no probe", but only for a probe the user could see:
// Defaults reports a TCP, gRPC or otherwise unmodelled probe as an empty path
// too, and deleting on that would throw away a running workload's own
// configuration because the reader did not recognize its shape.
func applyProbe(container map[string]any, draft Draft) bool {
	probe := mapAt(container, keyReadinessProbe)

	switch {
	case probe != nil && stringAt(probe, keyPath) == "":
		return false

	case !draft.WantsReadinessProbe():
		// Declined. The block goes rather than being left pointing at an empty
		// path, which is neither a probe nor an absence of one.
		delete(container, keyReadinessProbe)

	case probe != nil:
		probe[keyPath] = draft.healthPath()
		probe[keyPort] = draft.Port

	default:
		// Asked for on a workload that has none. Only the two fields the wizard
		// knows about are written; the timings are the platform's defaults,
		// which is what a probe added here has always meant.
		container[keyReadinessProbe] = map[string]any{
			keyPath: draft.healthPath(),
			keyPort: draft.Port,
		}
	}

	return true
}

// applyBuild sets the image source, editing the live build block in place
// rather than replacing it. The Build struct models only the fields the
// wizard asks about; a build block can carry more (build arguments, a
// timeout, whatever the platform adds next), and rebuilding it from the
// struct would silently drop all of it. Only the other two forms are removed,
// so the container never names an image and a way to build one at once.
func applyBuild(container map[string]any, build Build) error {
	switch build.Mode {
	case BuildModeUnchanged:
		// The live block says something this release does not model. Leaving
		// it exactly as it is beats rewriting it into the nearest thing the
		// Build struct can express.

	case BuildModeImage:
		delete(container, keyImageBuildConfig)

		container[keyImageURI] = build.ImageURI

	case BuildModeDockerfile:
		delete(container, keyImageURI)

		dockerfile := buildDockerfileBlock(container)
		dockerfile[keySource] = sourceProvided

		// The generated-only fields are meaningless under a provided
		// Dockerfile, so switching away from generated takes them with it.
		delete(dockerfile, keyExecEnvID)
		delete(dockerfile, keyExecEnvVersionID)
		delete(dockerfile, keyEntrypoint)

	case BuildModeGenerated:
		delete(container, keyImageURI)

		dockerfile := buildDockerfileBlock(container)
		dockerfile[keySource] = sourceGenerated
		dockerfile[keyExecEnvID] = build.ExecutionEnvironmentID
		dockerfile[keyExecEnvVersionID] = build.ExecutionEnvironmentVersionID
		dockerfile[keyEntrypoint] = toAnySlice(build.Entrypoint)

	default:
		return fmt.Errorf("unknown build mode %q", build.Mode)
	}

	return nil
}

// buildDockerfileBlock returns the container's build block to edit, creating
// the nesting only where the container had none.
func buildDockerfileBlock(container map[string]any) map[string]any {
	buildConfig := mapAt(container, keyImageBuildConfig)
	if buildConfig == nil {
		buildConfig = map[string]any{}
		container[keyImageBuildConfig] = buildConfig
	}

	dockerfile := mapAt(buildConfig, keyDockerfile)
	if dockerfile == nil {
		dockerfile = map[string]any{}
		buildConfig[keyDockerfile] = dockerfile
	}

	return dockerfile
}

// Render writes the live view as a manifest: the same file shape Draft
// produces, with the artifact spec and runtime blocks passed through as the
// platform gave them.
func (l Live) Render() ([]byte, error) {
	spec, err := documentNode(l.Spec)
	if err != nil {
		return nil, err
	}

	hoistNameKeys(spec)

	artifact := []field{{key: keyName, value: scalar(l.ArtifactName)}}
	if l.ArtifactType != "" {
		artifact = append(artifact, field{key: keyType, value: scalar(l.ArtifactType)})
	}

	if spec != nil {
		artifact = append(artifact, field{key: keySpec, value: spec})
	}

	fields := []field{
		{key: keyWorkloadID, value: scalar(l.WorkloadID), comment: workloadIDComment},
		{key: keyName, value: scalar(l.Name)},
		{
			key: keyImportance, value: scalar(orDefaultString(l.Importance, DefaultImportance)),
			comment: "low | moderate | high | critical",
		},
		{key: keyArtifact, value: mapping(artifact...)},
	}

	runtime, err := documentNode(l.Runtime)
	if err != nil {
		return nil, err
	}

	hoistNameKeys(runtime)

	if runtime != nil {
		fields = append(fields, field{key: keyRuntime, value: runtime})
	}

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(mapping(fields...)); err != nil {
		return nil, fmt.Errorf("cannot render manifest: %w", err)
	}

	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("cannot render manifest: %w", err)
	}

	return spaceTopLevelBlocks(buf.Bytes()), nil
}

// primaryContainer finds the container traffic reaches: the one flagged
// primary, falling back to the first container of the first group for a spec
// that flags nothing.
func (l Live) primaryContainer() map[string]any {
	return primaryContainerOf(l.Spec)
}

func primaryContainerOf(spec map[string]any) map[string]any {
	var first map[string]any

	for _, group := range slicesAt(spec, keyContainerGroups) {
		for _, container := range slicesAt(group, keyContainers) {
			if first == nil {
				first = container
			}

			if boolAt(container, keyPrimary) {
				return container
			}
		}
	}

	return first
}

// documentNode encodes a server document as a YAML node, nil for an empty
// one so the key is left out rather than written as an empty mapping.
func documentNode(document map[string]any) (*yaml.Node, error) {
	if len(document) == 0 {
		return nil, nil
	}

	node := &yaml.Node{}
	if err := node.Encode(document); err != nil {
		return nil, fmt.Errorf("cannot render the live spec: %w", err)
	}

	return node, nil
}

// hoistNameKeys walks the YAML node tree and moves any "name" key to the front of
// its mapping's Content, preserving every other key in its original order. It is
// applied to the nested spec and runtime blocks; the top-level manifest order is
// already controlled by Render's explicit field list.
func hoistNameKeys(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == keyName {
				nameKey := node.Content[i]
				nameValue := node.Content[i+1]

				copy(node.Content[2:i+2], node.Content[0:i])
				node.Content[0] = nameKey
				node.Content[1] = nameValue

				break
			}
		}
	}

	for _, child := range node.Content {
		hoistNameKeys(child)
	}
}

// stripServerManaged removes the server's bookkeeping from a copy of the
// document, at every level.
func stripServerManaged(document map[string]any) map[string]any {
	copied := deepCopyMap(document)
	stripKeys(copied)

	return copied
}

func stripKeys(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range serverManagedKeys {
			delete(typed, key)
		}

		// Normalize enabled: null inside a present autoscaling block before
		// the nil-purge would collapse it to {}. The platform's default for a
		// configured autoscaling block is ON, so enabled: null carries that
		// meaning; rewriting it to true keeps the rendered file free of nulls
		// (invariant 3) and semantically honest — both autoscalingActive and
		// checkScaling read the post-strip shape as ON.
		normalizeAutoscalingEnabled(typed)

		// Nil-valued keys are dropped in the same walk: the server echoes
		// them as placeholders (e.g. autoscaling: null), and writing one
		// into the committed manifest would confuse the validator. Empty
		// maps are preserved; isNullish is the validator's counterpart that
		// treats an empty mapping as absent rather than as a policy.
		for key, child := range typed {
			if child == nil {
				delete(typed, key)

				continue
			}

			stripKeys(child)
		}
	case []any:
		for _, item := range typed {
			stripKeys(item)
		}
	}
}

// normalizeAutoscalingEnabled rewrites enabled: null inside a present
// autoscaling block to enabled: true before the nil-purge would collapse it.
// The platform's default for a configured autoscaling block is ON, so an
// explicit enabled: null carries that meaning. Without this normalization the
// purge strips enabled: null, leaving {} — which both autoscalingActive and
// checkScaling read as OFF (absent), inverting the group's semantics.
//
// Only the nil placeholder is rewritten. A present bool (true or false) or an
// absent enabled key already carries the right meaning and is left untouched.
// A block like {enabled: null, minReplicaCount: 2} already reads as ON after
// the purge (enabled absent → default on); normalizing it to enabled: true
// makes the default explicit and keeps the rendered file free of nulls.
func normalizeAutoscalingEnabled(doc map[string]any) {
	autoscaling, ok := doc[keyAutoscaling].(map[string]any)
	if !ok {
		return
	}

	if autoscaling[keyEnabled] == nil {
		autoscaling[keyEnabled] = true
	}
}

// stripBuildOutputs removes what a build produced rather than what a build
// was asked to do. A container that says how to build itself also carries the
// image that came out and the code the last sync uploaded; both are re-earned
// by the next deploy, and writing them into the file would pin a workload to
// yesterday's image. It also strips the server-assigned build block, scoped
// to containers on purpose: the bare word "build" is generic enough that
// adding it to serverManagedKeys could eat a legitimate user key at some
// other level of the spec. The computed imageOutdated flag is unambiguous,
// so it lives in that list instead.
//
// An imageBuildConfig with real content (e.g. a dockerfile block) is a build
// container: imageUri is deleted as a server-built pin and codeRef as
// server-managed. An imageBuildConfig that is empty, or becomes empty after
// deleting codeRef, is NOT a build container — the user-declared imageUri is
// kept and the emptied imageBuildConfig key is removed entirely so it never
// renders as "{}" alongside a missing imageUri, which Validate would reject
// with "must set either imageUri or imageBuildConfig".
func stripBuildOutputs(spec map[string]any) {
	for _, group := range slicesAt(spec, keyContainerGroups) {
		for _, container := range slicesAt(group, keyContainers) {
			buildConfig := mapAt(container, keyImageBuildConfig)
			if buildConfig != nil {
				delete(buildConfig, "codeRef")

				// If the build config still has real content after stripping
				// codeRef, this is a genuine build container: drop the
				// server-built imageUri pin. If it emptied, the container is
				// really an image container — drop the emptied
				// imageBuildConfig key and keep imageUri.
				if len(buildConfig) == 0 {
					delete(container, keyImageBuildConfig)
				} else {
					delete(container, keyImageURI)
				}
			}

			delete(container, "build")
		}
	}
}

// deepCopyMap returns a deep copy of document so the caller's input is not
// mutated by the strip passes that run against it.
func deepCopyMap(document map[string]any) map[string]any {
	if document == nil {
		return nil
	}

	copied := make(map[string]any, len(document))
	for key, value := range document {
		copied[key] = deepCopyValue(value)
	}

	return copied
}

func deepCopyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCopyMap(typed)
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = deepCopyValue(item)
		}

		return copied
	default:
		return typed
	}
}

func stringAt(document map[string]any, key string) string {
	value, _ := document[key].(string)

	return value
}

// floatAt reads a number that arrived as JSON (always float64), as YAML (int
// for a whole number) or from a previous Apply (a Go int).
func floatAt(document map[string]any, key string) (float64, bool) {
	switch value := document[key].(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func boolAt(document map[string]any, key string) bool {
	value, _ := document[key].(bool)

	return value
}

// intAt reads a number that arrived through JSON, where every number is a
// float64, or through YAML, where a whole number is an int.
func intAt(document map[string]any, key string) (int, bool) {
	switch value := document[key].(type) {
	case int:
		return value, true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func mapAt(document map[string]any, key string) map[string]any {
	value, _ := document[key].(map[string]any)

	return value
}

func slicesAt(document map[string]any, key string) []map[string]any {
	items, _ := document[key].([]any)
	out := make([]map[string]any, 0, len(items))

	for _, item := range items {
		if typed, ok := item.(map[string]any); ok {
			out = append(out, typed)
		}
	}

	return out
}

func stringsAt(document map[string]any, key string) []string {
	items, _ := document[key].([]any)
	out := make([]string, 0, len(items))

	for _, item := range items {
		if typed, ok := item.(string); ok {
			out = append(out, typed)
		}
	}

	return out
}

func toAnySlice(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}

	return out
}

func orDefaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

// Autoscaled reports whether the workload's runtime group scales itself. A
// caller asking the user for a replica count needs to know: applyRuntime
// drops the answer while autoscaling is on, so the question would be one
// whose answer could not be used.
func (l Live) Autoscaled() bool {
	groups := slicesAt(l.Runtime, keyContainerGroups)
	if len(groups) == 0 {
		return false
	}

	return autoscalingActive(groups[0])
}
