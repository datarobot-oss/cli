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

package up

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/datarobot/cli/internal/workload/manifest"
)

// ErrNoManifest reports that no manifest was found on the walk upward from
// the starting directory. It is a sentinel rather than a failure because the
// two callers want opposite things from it: on a terminal `up` opens the
// setup wizard and carries on, while a non-interactive one stops and names
// `dr workload config`. Deciding which is the command's job, not this
// package's.
var ErrNoManifest = errors.New("no " + manifest.FileName + " manifest")

// Loaded is everything the deploy needs from disk, read and checked once.
type Loaded struct {
	// Path is the manifest itself.
	Path string

	// ProjectDir is the directory holding it, which is the root of the code
	// that gets synced and where the local state directory lives. It is not
	// necessarily the directory `up` was pointed at: the manifest is found by
	// walking upward, the same way git finds its repository, so running from
	// a subdirectory deploys the whole project rather than the fragment the
	// shell happened to be sitting in.
	ProjectDir string

	// Env is what a --import-env or --update-env run did to this file before
	// it was read, zero when neither was asked for.
	Env EnvEdit

	// Manifest is the parse tree, kept so later stages can ask it questions
	// without re-reading the file.
	Manifest *manifest.Manifest

	// Compiled is the create payload with the CLI's two conveniences resolved:
	// workloadId lifted out, and every dr-credential shorthand expanded.
	Compiled *manifest.Compiled
}

// WorkloadID is the binding the file carries, "" when the workload has not
// been created yet.
func (l Loaded) WorkloadID() string {
	return l.Compiled.WorkloadID
}

// Spec is the artifact spec the file asks for, in the same shape the live
// artifact comes back in, so the two can be compared directly. Reading it off
// the compiled payload rather than the file is what makes the comparison fair:
// the shorthand is already expanded, so a credential reference does not look
// like drift against the object form the platform stores.
//
// The type is removed rather than compared, because the live side never has
// one: the platform reads the discriminator off the artifact and pops any type
// sent inside the spec, so a spec that came back from the server carries only
// the rest. A file that wrote its type there would otherwise report it missing
// on every single run, mint a version and roll for it, and find it missing
// again the next time, which is a deploy loop with no state that can end it.
// ArtifactType below is what compares the type, from wherever the file put it,
// and the wizard's own writer drops it from this block for the same reason.
func (l Loaded) Spec() (map[string]any, error) {
	payload, err := l.payload()
	if err != nil {
		return nil, err
	}

	artifact, _ := payload["artifact"].(map[string]any)
	hoistArtifactType(artifact)

	spec, _ := artifact["spec"].(map[string]any)

	return spec, nil
}

// hoistArtifactType moves a type written inside the spec up beside it, and
// takes it out of the spec either way.
//
// It exists because the file may legitimately say the type in either place,
// while the platform only ever answers in one: it reads the discriminator on
// the way in from wherever it finds it, keeps it on the artifact, and returns
// a spec without it. Any comparison of the file's artifact block against the
// platform's own document has to reconcile that first, or the type reads as a
// field the live side is missing, on every run, with nothing a deploy can do
// to settle it: mint a version, roll it, find it missing again.
//
// The callers pass a freshly decoded copy, so this never edits the compiled
// request. What a create sends is untouched, which is what keeps the two
// placements equally valid on the way in.
func hoistArtifactType(artifact map[string]any) {
	spec, ok := artifact["spec"].(map[string]any)
	if !ok {
		return
	}

	within, _ := spec[keyType].(string)

	delete(spec, keyType)

	if within == "" {
		return
	}

	// Beside the spec wins when a file says both, because that is where the
	// platform keeps the answer.
	if beside, _ := artifact[keyType].(string); beside == "" {
		artifact[keyType] = within
	}
}

// ArtifactType is the kind of artifact the file asks for, "" when it names
// none.
//
// It is read from beside the spec and from inside it, because a file can
// legitimately have written it in either: the platform accepts both on the way
// in and keeps the result on the artifact, and the manifest ledger blesses
// neither placement over the other. This is the only place the answer is read,
// since Spec above removes it so the spec walk cannot see it, so missing one
// placement here would mean a file that names a type is treated as naming
// none, and the walk never reverts what the file leaves out.
//
// Beside the spec wins when a file says both, because that is where the
// platform keeps it and so where a reader checking the live artifact would
// look for the answer.
func (l Loaded) ArtifactType() (string, error) {
	payload, err := l.payload()
	if err != nil {
		return "", err
	}

	artifact, _ := payload["artifact"].(map[string]any)
	hoistArtifactType(artifact)

	artifactType, _ := artifact[keyType].(string)

	return artifactType, nil
}

// Runtime is the sizing half of the file, again in the live object's shape.
func (l Loaded) Runtime() (map[string]any, error) {
	payload, err := l.payload()
	if err != nil {
		return nil, err
	}

	runtime, _ := payload["runtime"].(map[string]any)

	return runtime, nil
}

// payload decodes the compiled request. It is decoded per call rather than
// cached because a plan asks for each half exactly once, and a cache here
// would be a mutable field on a value type that is copied freely.
func (l Loaded) payload() (map[string]any, error) {
	var payload map[string]any

	if err := json.Unmarshal(l.Compiled.Payload, &payload); err != nil {
		return nil, fmt.Errorf("cannot read the compiled manifest: %w", err)
	}

	return payload, nil
}

// Load finds the manifest at or above dir, then parses, validates and
// compiles it. Validation runs before anything reaches the network, so a
// mistyped key costs milliseconds rather than a build.
//
// A missing manifest comes back as ErrNoManifest. A malformed one comes back
// as *manifest.ValidationError, whose findings each carry the line they sit
// on, so the caller can print the file's own coordinates rather than a
// paraphrase.
func Load(dir string) (Loaded, error) {
	path, err := manifest.Locate(dir)
	if err != nil {
		if errors.Is(err, manifest.ErrNotFound) {
			// Locate's own message says the same thing in different words, so
			// it is restated here rather than nested, and the sentinel stays
			// the thing callers match on.
			return Loaded{}, fmt.Errorf("%w in %s or any parent directory", ErrNoManifest, dir)
		}

		return Loaded{}, err
	}

	m, err := manifest.Load(path)
	if err != nil {
		return Loaded{}, err
	}

	if err := m.Validate(); err != nil {
		return Loaded{}, err
	}

	compiled, err := m.Compile()
	if err != nil {
		return Loaded{}, err
	}

	return Loaded{
		Path:       path,
		ProjectDir: filepath.Dir(path),
		Manifest:   m,
		Compiled:   compiled,
	}, nil
}
