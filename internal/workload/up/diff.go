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
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/datarobot/cli/internal/workload/manifest"
)

// Change is one field the manifest asks for that the live object does not
// already agree with.
type Change struct {
	// Path is where the field sits, in the file's own spelling, with
	// name-keyed lists addressed by name rather than by index:
	// artifact.spec.containerGroups[default].containers[primary].port.
	// Built to be read by a person: names go in verbatim, so one carrying a
	// dot or a bracket parses as something else. Keys is the same location in
	// the form code should ask about.
	Path string

	// Keys is Path as the segments the walk descended through, each name-keyed
	// element contributing its name: {containerGroups, default, containers,
	// primary, port}. A rebuild is decided from these.
	Keys []string

	// Want is the value the manifest asks for.
	Want any

	// Have is the live value, nil when Absent.
	Have any

	// Absent distinguishes a field the live object does not carry at all
	// from one that carries a different value. The plan renders the first
	// as an addition and the second as a change, and a reader needs to see
	// the difference: adding a probe is not the same act as moving one.
	Absent bool
}

// String renders a change the way the plan prints it.
func (c Change) String() string {
	if c.Absent {
		return fmt.Sprintf("%s: %v", c.Path, format(c.Want))
	}

	return fmt.Sprintf("%s: %v -> %v", c.Path, format(c.Have), format(c.Want))
}

// Subset reports every leaf in want that the live side does not already
// match. It is deliberately one-directional: whatever the file says, `up`
// makes true, and whatever the file leaves out, `up` leaves alone.
//
// That asymmetry is the whole contract. A workload can carry a GPU bundle, a
// sidecar and an autoscaling policy that the manifest has never heard of, and
// none of it is drift. Deleting a line from the file stops managing a field;
// it does not revert it. And because the comparison runs against the live
// object rather than against a stored hash, an edit made in the UI shows up
// here and is restored on the next run, which a hash of the last applied
// spec could never see.
func Subset(want, have map[string]any) []Change {
	var changes []Change

	walk("", nil, want, have, true, &changes)

	return changes
}

// walk compares one node. present says whether have was actually there, so a
// key holding an explicit null is not confused with a key that is missing.
// path and keys are threaded together so neither can be rebuilt from the
// other after the fact.
func walk(path string, keys []string, want, have any, present bool, out *[]Change) {
	if !present {
		*out = append(*out, Change{Path: path, Keys: keys, Want: want, Absent: true})

		return
	}

	switch w := want.(type) {
	case map[string]any:
		walkMap(path, keys, w, have, out)
	case []any:
		walkList(path, keys, w, have, out)
	default:
		if !equalAt(path, want, have) {
			*out = append(*out, Change{Path: path, Keys: keys, Want: want, Have: have})
		}
	}
}

// walkMap recurses into an object, in key order so the plan reads the same
// way twice.
func walkMap(path string, keys []string, want map[string]any, have any, out *[]Change) {
	h, ok := have.(map[string]any)
	if !ok {
		*out = append(*out, Change{Path: path, Keys: keys, Want: want, Have: have})

		return
	}

	for _, key := range slices.Sorted(maps.Keys(want)) {
		hv, present := h[key]

		walk(join(path, key), descend(keys, key), want[key], hv, present, out)
	}
}

// walkList compares a list. The API's own lists come in two shapes and they
// need opposite treatment: containerGroups, containers and environmentVars
// are keyed by name and reorder freely, so matching them by index would
// report a reordered file as a wholesale rewrite. Everything else, a
// resourceBundles of plain strings or an autoscaling policy with no name to
// key on, is compared whole, because a partial match of an unkeyed list
// means nothing.
func walkList(path string, keys []string, want []any, have any, out *[]Change) {
	h, ok := have.([]any)
	if !ok {
		*out = append(*out, Change{Path: path, Keys: keys, Want: want, Have: have})

		return
	}

	if !nameKeyed(want) {
		if !equal(want, h) {
			*out = append(*out, Change{Path: path, Keys: keys, Want: want, Have: h})
		}

		return
	}

	live := byName(h)

	for _, item := range want {
		element, _ := item.(map[string]any)
		name, _ := element["name"].(string)
		at := fmt.Sprintf("%s[%s]", path, name)
		below := descend(keys, name)

		counterpart, found := live[name]
		if !found {
			*out = append(*out, Change{Path: at, Keys: below, Want: element, Absent: true})

			continue
		}

		walkMap(at, below, element, counterpart, out)
	}
}

// descend is keys with one more segment, always in storage of its own:
// appending in place would let two siblings share a backing array.
func descend(keys []string, key string) []string {
	return append(slices.Clone(keys), key)
}

// nameKeyed reports whether every element is an object carrying a non-empty
// name, which is what makes matching by name safe. An empty list is not
// name-keyed: there is nothing to key on, and comparing it whole is right.
func nameKeyed(list []any) bool {
	if len(list) == 0 {
		return false
	}

	for _, item := range list {
		element, ok := item.(map[string]any)
		if !ok {
			return false
		}

		if name, ok := element["name"].(string); !ok || name == "" {
			return false
		}
	}

	return true
}

// byName indexes a live list so want's elements can find their counterparts.
// Elements without a usable name are skipped rather than guessed at; they
// simply have nothing for want to match, and want's own element then reads
// as an addition.
func byName(list []any) map[string]map[string]any {
	indexed := make(map[string]map[string]any, len(list))

	for _, item := range list {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}

		if name, ok := element["name"].(string); ok && name != "" {
			indexed[name] = element
		}
	}

	return indexed
}

// equal compares two decoded JSON values. Both sides reach here through
// encoding/json, so numbers are already float64 on both and reflect is
// enough; there is no int-versus-float mismatch to normalise away.
func equal(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// memoryField is the one leaf whose two sides are written differently: the
// file says "512MB" and the platform stores and returns 512000000.
const memoryField = ".memory"

// equalAt compares a leaf, knowing which field it is.
//
// Only a size is compared loosely, and only where the path says the leaf is
// one. Without this the same sizing written two ways reads as drift on every
// run, for ever, so a workload can never report itself up to date; with it
// applied everywhere, an unrelated numeric string would quietly compare equal
// to its number.
func equalAt(path string, a, b any) bool {
	if equal(a, b) {
		return true
	}

	return strings.HasSuffix(path, memoryField) && sameMemory(a, b)
}

// sameMemory reports whether two values are the same size written differently.
// It answers false unless both sides parse as a size, so nothing else is
// compared loosely by accident.
func sameMemory(a, b any) bool {
	left, ok := memoryBytes(a)
	if !ok {
		return false
	}

	right, ok := memoryBytes(b)

	return ok && left == right
}

// memoryBytes reads a size from either shape the two sides use: the file's
// string, or the platform's number.
func memoryBytes(v any) (int64, bool) {
	switch typed := v.(type) {
	case string:
		return manifest.MemoryBytes(typed)
	case float64:
		// JSON numbers arrive as float64. A size is a whole number of bytes,
		// so anything fractional is not one.
		if typed != math.Trunc(typed) {
			return 0, false
		}

		return int64(typed), true
	default:
		return 0, false
	}
}

// join builds a dotted path, skipping the leading dot at the root.
func join(path, key string) string {
	if path == "" {
		return key
	}

	return path + "." + key
}

// format renders a value for the plan. Composite values are summarised
// rather than dumped: a plan is a summary, and a reader who wants the whole
// object has the file open next to it.
func format(v any) string {
	switch value := v.(type) {
	case nil:
		return "unset"
	case string:
		return value
	case map[string]any:
		return "{" + strings.Join(slices.Sorted(maps.Keys(value)), ", ") + "}"
	case []any:
		return fmt.Sprintf("[%d item(s)]", len(value))
	default:
		return fmt.Sprintf("%v", value)
	}
}

// Extra reports every element of a name-keyed list that have carries and want
// does not name, as paths.
//
// It is the question Subset deliberately does not ask. Measuring drift against
// a running workload is one-directional on purpose: a field the file leaves
// out is left alone rather than reverted. But "could this document have been
// produced by this file?" is a different question, and there a name the file
// never mentions is proof that it could not, because creating it fresh would
// not have put that name there.
//
// Only names are compared, never the keys inside an element. The platform
// fills in its own defaults and server-owned fields, so an element the file
// does name always carries more than the file said and none of that is
// evidence of anything. The name-keyed lists are different: containerGroups,
// containers and environmentVars are the file's own, and the platform adds no
// entries to them.
func Extra(want, have map[string]any) []string {
	var out []string

	extra("", want, have, &out)

	return out
}

// extra walks have alongside want. It iterates have's keys rather than want's,
// because the case worth catching is a list the file no longer mentions at
// all: deleting the last environment variable drops the whole block from the
// file, and a walk driven by want would never look at it.
func extra(path string, want map[string]any, have map[string]any, out *[]string) {
	for _, key := range slices.Sorted(maps.Keys(have)) {
		at := join(path, key)

		if list, ok := have[key].([]any); ok && nameKeyed(list) {
			extraInList(at, want[key], list, out)

			continue
		}

		child, ok := have[key].(map[string]any)
		if !ok {
			continue
		}

		wanted, ok := want[key].(map[string]any)
		if !ok {
			// The file says nothing about this object, so nothing inside it
			// can be something the file removed.
			continue
		}

		extra(at, wanted, child, out)
	}
}

// extraInList compares one name-keyed list. A want side that is missing or is
// not a list reads as empty, which is exactly what deleting the block means.
func extraInList(path string, want any, have []any, out *[]string) {
	named := map[string]map[string]any{}

	if list, ok := want.([]any); ok {
		named = byName(list)
	}

	for _, item := range have {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name, _ := element["name"].(string)
		at := fmt.Sprintf("%s[%s]", path, name)

		counterpart, found := named[name]
		if !found {
			*out = append(*out, at)

			continue
		}

		extra(at, counterpart, element, out)
	}
}
