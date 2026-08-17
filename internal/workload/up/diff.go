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
	"reflect"
	"slices"
	"strings"
)

// Change is one field the manifest asks for that the live object does not
// already agree with.
type Change struct {
	// Path is where the field sits, in the file's own spelling, with
	// name-keyed lists addressed by name rather than by index:
	// artifact.spec.containerGroups[default].containers[primary].port.
	Path string

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

	walk("", want, have, true, &changes)

	return changes
}

// walk compares one node. present says whether have was actually there, so a
// key holding an explicit null is not confused with a key that is missing.
func walk(path string, want, have any, present bool, out *[]Change) {
	if !present {
		*out = append(*out, Change{Path: path, Want: want, Absent: true})

		return
	}

	switch w := want.(type) {
	case map[string]any:
		walkMap(path, w, have, out)
	case []any:
		walkList(path, w, have, out)
	default:
		if !equal(want, have) {
			*out = append(*out, Change{Path: path, Want: want, Have: have})
		}
	}
}

// walkMap recurses into an object, in key order so the plan reads the same
// way twice.
func walkMap(path string, want map[string]any, have any, out *[]Change) {
	h, ok := have.(map[string]any)
	if !ok {
		*out = append(*out, Change{Path: path, Want: want, Have: have})

		return
	}

	for _, key := range slices.Sorted(maps.Keys(want)) {
		hv, present := h[key]

		walk(join(path, key), want[key], hv, present, out)
	}
}

// walkList compares a list. The API's own lists come in two shapes and they
// need opposite treatment: containerGroups, containers and environmentVars
// are keyed by name and reorder freely, so matching them by index would
// report a reordered file as a wholesale rewrite. Everything else, a
// resourceBundles of plain strings or an autoscaling policy with no name to
// key on, is compared whole, because a partial match of an unkeyed list
// means nothing.
func walkList(path string, want []any, have any, out *[]Change) {
	h, ok := have.([]any)
	if !ok {
		*out = append(*out, Change{Path: path, Want: want, Have: have})

		return
	}

	if !nameKeyed(want) {
		if !equal(want, h) {
			*out = append(*out, Change{Path: path, Want: want, Have: h})
		}

		return
	}

	live := byName(h)

	for _, item := range want {
		element, _ := item.(map[string]any)
		name, _ := element["name"].(string)
		at := fmt.Sprintf("%s[%s]", path, name)

		counterpart, found := live[name]
		if !found {
			*out = append(*out, Change{Path: at, Want: element, Absent: true})

			continue
		}

		walkMap(at, element, counterpart, out)
	}
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
