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
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"gopkg.in/yaml.v3"
)

// Workload kinds the setup wizard can write. The full set the format accepts
// is larger (stacks, NIMs); those are hand-written or added by later wizard
// tracks, and pass through the reader either way.
const (
	TypeService = "service"
	TypeAgent   = "agent"
)

// ArtifactTypeOrDefault applies the platform's default for an artifact block
// that names no type: it is created as a service. Every reader that has to
// predict what kind of thing the platform is about to make has to say so the
// same way, or two of them answer differently about one file.
func ArtifactTypeOrDefault(artifactType string) string {
	return orDefaultString(artifactType, TypeService)
}

// SameArtifactType reports whether two artifact types name the same kind.
//
// Folded because the platform's own documents disagree about the casing of
// their enums. Comparing them exactly is what turns a hand-written "Service"
// into drift that can never be satisfied: nothing about the file changes it,
// so every run mints a version and rolls it, for ever, against a workload
// nobody touched.
func SameArtifactType(a, b string) bool {
	return strings.EqualFold(a, b)
}

// Build modes, named the way the user chooses them rather than the way the
// spec spells them: BuildModeDockerfile writes source: provided.
const (
	BuildModeDockerfile = "dockerfile"
	BuildModeGenerated  = "generated"
	BuildModeImage      = "image"

	// BuildModeUnchanged means the live container's build block says
	// something this release does not model, so it is carried over verbatim
	// instead of being rewritten. It only ever comes from a live workload;
	// Draft.Render rejects it, because there is nothing to render from.
	BuildModeUnchanged = "unchanged"
)

// Importance levels, in the order the platform ranks them. The enum itself
// is not verified against a running platform, so the reader does not enforce
// it: an unrecognized value passes through and the server decides.
var ImportanceLevels = []string{"low", "moderate", "high", "critical"}

// ValidImportance reports whether value is one of ImportanceLevels, for a
// caller that would rather reject a typo at the prompt than at the API.
func ValidImportance(value string) bool {
	return slices.Contains(ImportanceLevels, value)
}

const (
	DefaultImportance = "low"
	DefaultPort       = 8080
	// DefaultHealthPath is the root rather than /health because the probe has
	// to pass on an application nobody wrote it for. Almost every HTTP service
	// answers something at /, where /health is an endpoint the app has to have
	// implemented; pointing a probe at one that does not exist is the most
	// common first-deploy failure, and it presents as a deploy that never
	// becomes ready rather than as a misconfigured probe. An app with a real
	// health endpoint says so on the settings screen, and one that wants no
	// probe at all clears the field.
	DefaultHealthPath = "/"
	DefaultReplicas   = 1
	DefaultCPU        = 0.5
	DefaultMemory     = "512MB"

	// GroupName and PrimaryContainerName appear in both halves of the file,
	// which is how the sizing block is joined to the artifact block.
	GroupName            = "default"
	PrimaryContainerName = "primary"

	// ArtifactNameSuffix derives the artifact name from the workload name, so
	// the two are recognizably one project in listings.
	ArtifactNameSuffix = "-artifact"
)

// atomicWrite is the durable write, a variable so a test can make it fail and
// check that the reservation is taken back.
var atomicWrite = fsutil.AtomicWriteFile

// ErrExists reports that a manifest is already present at the target path.
// Setup never rewrites one: the file is the interface after the first write,
// and every later change is a hand edit.
var ErrExists = errors.New(FileName + " already exists")

// Draft is what the setup wizard settles: the fixed subset of the spec a
// generated file spells out. It is not a spec struct. Reading stays
// node-based so unknown blocks survive a round trip, and a hand-edited file
// can say things no Draft field can express.
type Draft struct {
	// WorkloadID binds the file to a live workload; empty means the workload
	// is created by the first up.
	WorkloadID string
	Name       string
	Importance string
	// Type is TypeService or TypeAgent.
	Type string
	// A2AEnabled publishes an agent's A2A card to the tenant-wide registry.
	// Written only when true, because false is the platform's own default.
	A2AEnabled bool
	Build      Build
	// Port is the primary container's port, and the readiness probe's.
	Port int
	// HealthPath is the readiness probe's path, and empty means the workload
	// runs without a probe: the platform does not require one, and a probe
	// aimed at an endpoint the container does not serve never passes. Read it
	// through WantsReadinessProbe rather than comparing it here and there.
	HealthPath string
	Runtime    Runtime
	// EnvVars are the variables a local .env defines, already classified by
	// the caller. They are written into environmentVars: an ordinary value as
	// the literal it is, a secret as a credential reference carrying
	// CredentialPlaceholder, because no credential exists for it yet. An
	// empty slice writes nothing.
	EnvVars []EnvVar
}

// WantsReadinessProbe reports whether the answers ask for a probe at all. It
// is the one place the empty-path convention is spelled out, so the write, the
// live merge and the screens cannot drift about what an empty path means.
func (d Draft) WantsReadinessProbe() bool {
	return strings.TrimSpace(d.HealthPath) != ""
}

// EnvVar is one .env variable as the manifest will carry it. Deciding which
// are secret is the caller's job, because that needs the values.
type EnvVar struct {
	Name string
	// Value is written as a literal, and is empty for a secret: a secret's
	// value belongs in the credential store and never in this file.
	Value string
	// Secret means the value belongs in the credential store, so the entry is
	// written as a reference rather than as a literal.
	Secret bool
	// CredentialID is the credential holding the secret, when one was stored
	// for it. Empty means none was, and the reference is written carrying
	// CredentialPlaceholder instead: an entry that is visibly unfinished, which
	// a deploy refuses by name.
	CredentialID string
}

// credentialID is the id to write for a secret, and the placeholder when the
// secret never reached the credential store.
func (v EnvVar) credentialID() string {
	if v.CredentialID == "" {
		return CredentialPlaceholder
	}

	return v.CredentialID
}

// CredentialPlaceholder stands in for a credential id until one is created.
// It is written in full rather than left as a comment: a reference that is
// visibly unfinished fails at deploy naming the variable, where a commented
// entry would deploy cleanly and leave the container without its secret.
const CredentialPlaceholder = "PLACEHOLDER"

// credentialKey is the field of a credential a reference reads. Every
// placeholder names the same one, because swapping the id for a real one is
// then the only edit needed.
const credentialKey = "apiToken"

// Build is how the image comes to be: exactly one of the three modes, with
// the fields that mode needs.
type Build struct {
	Mode string
	// ImageURI is set for BuildModeImage.
	ImageURI string
	// The rest are set for BuildModeGenerated. Both environment ids are
	// recorded, resolved once at setup, so builds stay reproducible after the
	// environment moves on.
	ExecutionEnvironmentID        string
	ExecutionEnvironmentVersionID string
	Entrypoint                    []string
}

// Runtime is the sizing half: tunable at any moment with no new artifact
// version and no rebuild.
type Runtime struct {
	Replicas int
	CPU      float64
	Memory   string
}

// Path returns the manifest path for a project directory.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// ArtifactName returns the artifact name derived from a workload name.
func ArtifactName(workloadName string) string {
	return workloadName + ArtifactNameSuffix
}

// Render turns a draft into the bytes of a manifest: the platform's own spec,
// commented where a reader would otherwise have to look something up. Every
// field the wizard settles is written explicitly rather than left to a
// reader's defaults, so the file means the same thing to this CLI and to
// push-to-deploy.
//
// Render does not validate. Parse the result and call Validate, which is what
// the wizard does before writing anything.
func (d Draft) Render() ([]byte, error) {
	build, err := d.buildFields()
	if err != nil {
		return nil, err
	}

	root := mapping(d.topLevelFields(build)...)

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("cannot render manifest: %w", err)
	}

	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("cannot render manifest: %w", err)
	}

	return spaceTopLevelBlocks(buf.Bytes()), nil
}

// Write creates the manifest at path. It refuses to overwrite an existing
// one, and no caller ever sees a truncated manifest: the content arrives
// through an atomic rename, so the file either has all of it or does not
// exist, and a write that fails takes its own reservation back.
//
// The exclusive create is what makes the refusal a check rather than a race:
// two processes configuring the same directory cannot both believe they wrote
// the file. It is a reservation, immediately released, so the content still
// arrives through the atomic rename.
//
// The one residue this cannot clean up is a crash between the two steps,
// which leaves the empty reservation on disk. Collapsing them into a single
// rename would fix that and lose the exclusive create, trading a file Parse
// rejects by name for the chance of silently replacing a manifest someone
// else just wrote. The empty file is the better failure, so it is the one
// kept, and Parse names the fix.
func Write(path string, data []byte) error {
	reservation, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fsutil.DefaultFileMode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrExists, path)
		}

		return fmt.Errorf("cannot create %s: %w", path, err)
	}

	_ = reservation.Close()

	if err := atomicWrite(path, data); err != nil {
		// The reservation is ours and still empty, so taking it back leaves
		// the directory as it was found.
		_ = os.Remove(path)

		return err
	}

	return nil
}

// WriteWorkloadID records the binding in an existing manifest, the one edit
// the CLI makes to a file the user owns: up writes the id back the moment the
// workload exists, so the next run finds it. Comments, unknown keys and their
// order are preserved, because the file is re-emitted from the tree it was
// parsed into rather than regenerated.
//
// This is the one way into the package that does not go through Parse, so it
// runs the alias guard itself. Reaching here means the file already parsed
// once, and neither the top-level walk below nor the encoder expands an alias,
// so nothing known today needs the check. It is here because that reasoning is
// about the current implementation of two other functions, and a guard that
// holds only while nobody edits them is not one.
func WriteWorkloadID(path, workloadID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	var doc yaml.Node

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("cannot parse %s: %w", path, err)
	}

	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("cannot record the workload id in %s: root must be a YAML mapping", path)
	}

	if err := checkAliases(doc.Content[0]); err != nil {
		return fmt.Errorf("cannot record the workload id in %s: %w", path, err)
	}

	setWorkloadID(doc.Content[0], workloadID)

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("cannot render %s: %w", path, err)
	}

	if err := enc.Close(); err != nil {
		return fmt.Errorf("cannot render %s: %w", path, err)
	}

	return fsutil.AtomicWriteFile(path, buf.Bytes())
}

// setWorkloadID replaces the binding in place when the key is there, and
// otherwise inserts it as the first key. A head comment on what used to be
// the first key is a file banner, so it moves up with the insertion instead
// of ending up stranded in the middle of the file.
func setWorkloadID(root *yaml.Node, workloadID string) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == keyWorkloadID {
			root.Content[i+1].SetString(workloadID)

			return
		}
	}

	key := scalar(keyWorkloadID)
	key.LineComment = workloadIDComment

	if len(root.Content) > 0 {
		key.HeadComment = root.Content[0].HeadComment
		root.Content[0].HeadComment = ""
	}

	root.Content = append([]*yaml.Node{key, scalar(workloadID)}, root.Content...)
}

const workloadIDComment = "managed by the CLI"

func (d Draft) topLevelFields(build field) []field {
	fields := make([]field, 0, 5)

	if d.WorkloadID != "" {
		fields = append(fields, field{key: keyWorkloadID, value: scalar(d.WorkloadID), comment: workloadIDComment})
	}

	importance := d.Importance
	if importance == "" {
		importance = DefaultImportance
	}

	fields = append(fields,
		field{key: keyName, value: scalar(d.Name)},
		field{key: keyImportance, value: scalar(importance), comment: strings.Join(ImportanceLevels, " | ")},
		field{key: keyArtifact, value: d.artifact(build)},
		field{key: keyRuntime, value: d.runtime()},
	)

	return fields
}

// artifact renders the half that says what runs: the versioned, lockable
// definition.
func (d Draft) artifact(build field) *yaml.Node {
	// type sits on the artifact, not in the spec. The platform pops any
	// spec.type it is sent and re-derives the discriminator from the
	// artifact's own type, defaulting to service, so a type written one level
	// down is discarded without a word: an agent silently becomes a service,
	// and a2aEnabled is then rejected as an unknown field on the service
	// variant.
	spec := []field{}

	if d.A2AEnabled {
		spec = append(spec, field{
			key:     keyA2AEnabled,
			value:   boolean(true),
			comment: "discoverable tenant-wide",
		})
	}

	container := []field{
		{key: keyName, value: scalar(PrimaryContainerName)},
		{key: keyPrimary, value: boolean(true)},
		{key: keyPort, value: number(d.Port), comment: "must be 1024 or above"},
		build,
	}

	if d.WantsReadinessProbe() {
		container = append(container, field{key: keyReadinessProbe, value: mapping(
			field{key: keyPath, value: scalar(d.HealthPath)},
			field{key: keyPort, value: number(d.Port)},
		)})
	}

	if vars := d.environmentVars(); vars != nil {
		container = append(container, field{key: keyEnvironmentVars, value: vars})
	}

	spec = append(spec, field{key: keyContainerGroups, value: sequence(mapping(
		field{key: keyName, value: scalar(GroupName)},
		field{key: keyContainers, value: sequence(mapping(container...))},
	))})

	return mapping(
		field{key: keyName, value: scalar(ArtifactName(d.Name))},
		field{key: keyType, value: scalar(d.Type)},
		field{key: keySpec, value: mapping(spec...)},
	)
}

// runtime renders the half that says how much it runs with.
func (d Draft) runtime() *yaml.Node {
	allocation := mapping(
		field{key: keyCPU, value: decimal(d.Runtime.CPU)},
		field{key: keyMemory, value: scalar(d.Runtime.Memory)},
	)
	allocation.Style = yaml.FlowStyle

	return mapping(field{key: keyContainerGroups, value: sequence(mapping(
		field{key: keyName, value: scalar(GroupName), comment: "matches the artifact group"},
		field{key: keyReplicaCount, value: number(d.Runtime.Replicas)},
		field{key: keyContainers, value: sequence(mapping(
			field{key: keyName, value: scalar(PrimaryContainerName)},
			field{key: keyResourceAllocation, value: allocation},
		))},
	))})
}

// environmentVars renders the .env variables the manifest carries. Ordinary
// settings become literals. A secret becomes a credential reference: to the
// credential that was stored for it, or, when none was, to
// CredentialPlaceholder, so the entry is present and obviously unfinished
// rather than absent and easy to miss.
func (d Draft) environmentVars() *yaml.Node {
	entries := sequence()

	for _, v := range d.EnvVars {
		value := scalar(v.Value)
		entry := []field{{key: keyName, value: scalar(v.Name)}}

		switch {
		case v.Secret && v.CredentialID == "":
			value = scalar(CredentialShorthandPrefix + CredentialPlaceholder + "/" + credentialKey)
			entry = append(entry, field{
				key:     keyValue,
				value:   value,
				comment: "replace " + CredentialPlaceholder + " with the credential id",
			})

		case v.Secret:
			value = scalar(CredentialShorthandPrefix + v.credentialID() + "/" + credentialKey)
			entry = append(entry, field{key: keyValue, value: value})

		default:
			entry = append(entry, field{key: keyValue, value: value})
		}

		entries.Content = append(entries.Content, mapping(entry...))
	}

	if len(entries.Content) == 0 {
		return nil
	}

	return entries
}

// buildFields renders the one key that says where the image comes from.
func (d Draft) buildFields() (field, error) {
	switch d.Build.Mode {
	case BuildModeDockerfile:
		return field{
			key: keyImageBuildConfig,
			value: mapping(field{key: keyDockerfile, value: mapping(
				field{key: keySource, value: scalar(sourceProvided)},
			)}),
			comment: "built from this repository's " + dockerfileName,
		}, nil

	case BuildModeGenerated:
		entrypoint := sequence()

		for _, arg := range d.Build.Entrypoint {
			// Quote every argument, including the ones that would be legal
			// bare: an entrypoint reads as a command line, and 0.0.0.0 or
			// 8080 sitting unquoted next to quoted neighbours invites an
			// editor to wonder which ones are strings.
			arg := scalar(arg)
			arg.Style = yaml.DoubleQuotedStyle

			entrypoint.Content = append(entrypoint.Content, arg)
		}

		entrypoint.Style = yaml.FlowStyle

		return field{
			key: keyImageBuildConfig,
			value: mapping(field{key: keyDockerfile, value: mapping(
				field{key: keySource, value: scalar(sourceGenerated)},
				field{key: keyExecEnvID, value: scalar(d.Build.ExecutionEnvironmentID)},
				field{key: keyExecEnvVersionID, value: scalar(d.Build.ExecutionEnvironmentVersionID)},
				field{key: keyEntrypoint, value: entrypoint},
			)}),
			comment: "generated from an execution environment",
		}, nil

	case BuildModeImage:
		return field{key: keyImageURI, value: scalar(d.Build.ImageURI), comment: "already published; no build"}, nil

	case BuildModeUnchanged:
		// Only a live workload can carry this, and that path renders through
		// Live, which keeps the block it came from.
		return field{}, errors.New("cannot render a build mode of " + BuildModeUnchanged + " from scratch")

	default:
		return field{}, fmt.Errorf("unknown build mode %q: want %s, %s or %s",
			d.Build.Mode, BuildModeDockerfile, BuildModeGenerated, BuildModeImage)
	}
}

// field is one key/value pair of a rendered mapping, with the comment that
// earns its place next to it.
type field struct {
	key   string
	value *yaml.Node
	// comment sits at the end of the key's own line.
	comment string
}

func mapping(fields ...field) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	for _, f := range fields {
		key := scalar(f.key)
		key.LineComment = f.comment

		node.Content = append(node.Content, key, f.value)
	}

	return node
}

func sequence(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

// yaml11Booleans are the plain scalars a YAML 1.1 reader resolves to
// booleans. yaml.v3 follows YAML 1.2 and leaves them as strings, so it sees
// no reason to quote them, but this file is read by more than one parser and
// a workload named "no" must not arrive at the platform as false.
var yaml11Booleans = map[string]bool{"y": true, "yes": true, "n": true, "no": true, "on": true, "off": true}

// scalar renders a string, quoting it when a plain scalar would read back as
// something other than this string. The encoder already handles the YAML 1.2
// cases (a name of "8080" or "null" comes back quoted); the map above covers
// what only older readers would misread. "my-app" stays bare.
func scalar(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if yaml11Booleans[strings.ToLower(value)] {
		node.Style = yaml.DoubleQuotedStyle
	}

	return node
}

func number(value int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: intTag, Value: strconv.Itoa(value)}
}

// decimal renders a CPU allocation. A whole number is written plainly (1, not
// 1.0 and not the !!float tag yaml.v3 would otherwise emit to keep the type):
// the spec's examples write cpu: 1, and a tagged scalar in a generated file
// reads like a mistake.
//
// Whole-ness is decided on the formatted digits rather than on the float,
// which is what keeps this free of a magnitude bound. FormatFloat with -1
// precision prints the shortest text that reads back as exactly this value, so
// "no decimal point in it" is the entire test, and no int64 conversion happens
// whose range would then have to be guarded. NaN and ±Inf format as words
// instead of digits and so take the float branch, which is where they were
// before: a cpu that is not a number is the flag layer's problem, and this
// function should not launder it into a plausible-looking integer.
func decimal(value float64) *yaml.Node {
	text := strconv.FormatFloat(value, 'f', -1, 64)
	if strings.Contains(text, ".") || math.IsNaN(value) || math.IsInf(value, 0) {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: text}
	}

	return &yaml.Node{Kind: yaml.ScalarNode, Tag: intTag, Value: text}
}

func boolean(value bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: boolTag, Value: strconv.FormatBool(value)}
}

// blockKeys are the top-level keys that open a nested block. The encoder
// packs the whole document together; a blank line before each of these is
// what makes the two halves of the file readable at a glance.
var blockKeys = []string{keyArtifact + ":", keyRuntime + ":"}

func spaceTopLevelBlocks(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines)+len(blockKeys))

	for _, line := range lines {
		if opensBlock(line) && len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}

		out = append(out, line)
	}

	return []byte(strings.Join(out, "\n"))
}

func opensBlock(line string) bool {
	for _, key := range blockKeys {
		if strings.HasPrefix(line, key) {
			return true
		}
	}

	return false
}

// ResolveCredentials rewrites every credential reference still carrying
// CredentialPlaceholder to the id stored for that variable, keyed by ids on
// the environment variable's name.
//
// This exists for one case: the setup wizard's confirm screen lets the file be
// edited by hand before it is written, and the credentials are stored after
// that, deliberately, so walking to the end and cancelling leaves nothing
// behind. Re-rendering from the draft at that point would produce a file
// carrying the new ids and none of the user's edit. Editing their own bytes
// keeps both.
//
// Matching is by variable name because the placeholder text is identical for
// every secret in the file: two of them differ only in which entry they sit
// in, so a textual substitution could not tell them apart. A name the map does
// not mention is left alone, placeholder and all, which is what an entry no
// credential was stored for should look like.
func ResolveCredentials(content []byte, ids map[string]string) ([]byte, error) {
	if len(ids) == 0 {
		return content, nil
	}

	var doc yaml.Node

	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse the manifest to record the credential ids: %w", err)
	}

	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("cannot record the credential ids: root must be a YAML mapping")
	}

	if err := checkAliases(doc.Content[0]); err != nil {
		return nil, fmt.Errorf("cannot record the credential ids: %w", err)
	}

	setCredentialIDs(doc.Content[0], ids)

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("cannot render the manifest: %w", err)
	}

	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("cannot render the manifest: %w", err)
	}

	return buf.Bytes(), nil
}

// setCredentialIDs walks every environmentVars sequence in the tree, wherever
// the spec puts it, and fills in the placeholders it finds.
func setCredentialIDs(node *yaml.Node, ids map[string]string) {
	node = resolveAlias(node)
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == keyEnvironmentVars {
				fillPlaceholders(node.Content[i+1], ids)
			}

			setCredentialIDs(node.Content[i+1], ids)
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			setCredentialIDs(item, ids)
		}
	case yaml.DocumentNode, yaml.ScalarNode, yaml.AliasNode:
		// Leaves. AliasNode is unreachable: resolveAlias replaced it above.
	}
}

// fillPlaceholders rewrites one environmentVars sequence.
func fillPlaceholders(vars *yaml.Node, ids map[string]string) {
	placeholder := CredentialShorthandPrefix + CredentialPlaceholder + "/"

	for _, entry := range seqItems(vars) {
		name, ok := scalarString(mapValue(entry, keyName))
		if !ok {
			continue
		}

		id, ok := ids[name]
		if !ok || id == "" {
			continue
		}

		value := mapValue(entry, keyValue)
		if value == nil {
			continue
		}

		current, ok := scalarString(value)
		if !ok || !strings.HasPrefix(current, placeholder) {
			continue
		}

		value.SetString(CredentialShorthandPrefix + id + "/" + strings.TrimPrefix(current, placeholder))

		// The comment told the reader to replace the placeholder, which has
		// now happened.
		value.LineComment = ""
	}
}
