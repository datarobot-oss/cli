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
	"strings"
)

// location is one change reduced to what the printed plan says about it: where
// it sits, and a name for the field that moved.
//
// Every path the walk produces threads through containerGroups and containers
// before it reaches anything a person wrote, so a line that spells the whole
// path out is mostly scaffolding. What the reader is looking for sits at the
// end of it, and on any terminal narrower than the path the line wraps and
// buries exactly that. The whole path is still what the JSON envelope carries,
// for anything that has to act on it rather than read it.
type location struct {
	group     string
	container string

	// field is what changed, spelled the way the plan prints it.
	field string
}

// locate reduces one change.
//
// It reads the walk's key segments rather than the rendered path, for the same
// reason the rebuild decision does: a container whose name holds a bracket
// renders a path that cannot be parsed back into the names that built it. A
// change carrying no keys keeps its whole path, which is all that can honestly
// be said about one.
func locate(c Change) location {
	if len(c.Keys) == 0 {
		return location{field: c.Path}
	}

	var (
		at   location
		keys = c.Keys
	)

	if len(keys) >= 2 && keys[0] == keyContainerGroups {
		at.group = keys[1]
		keys = keys[2:]
	}

	if len(keys) >= 2 && keys[0] == keyContainers {
		at.container = keys[1]
		keys = keys[2:]
	}

	// Nothing sits under it, so the element itself is what the change is
	// about. Its name goes in the field rather than the scope column, which is
	// dropped when every change shares it and would leave the line naming
	// nothing at all.
	if len(keys) == 0 {
		return location{field: elementName(at)}
	}

	at.field = fieldName(keys)

	return at
}

// fieldName names what changed inside the container it belongs to.
func fieldName(keys []string) string {
	// An environment variable's own name is what the reader came for, and
	// "env" is enough to say which list it came from. The leaf below it, the
	// literal or the credential id that replaced it, is not printed: both are
	// redacted to one verb anyway, so it would add a segment and no fact.
	if len(keys) >= 2 && keys[0] == keyEnvironmentVars {
		return "env " + keys[1]
	}

	return strings.Join(keys, ".")
}

// elementName names a whole container, or a whole group, that the live
// workload does not carry at all.
func elementName(at location) string {
	if at.container != "" {
		return "container " + at.container
	}

	return "container group " + at.group
}

// scope is what the plan prints to the left of a change, empty for a change
// that sits outside any group.
//
// The group's name joins the container's only when the changes span more than
// one group, which is the one case where a container name is not enough to
// tell two lines apart.
func (at location) scope(withGroup bool) string {
	switch {
	case at.container == "":
		return at.group
	case withGroup:
		return at.group + "/" + at.container
	default:
		return at.container
	}
}

// shortLines renders a set of changes together, because what one line may
// leave out depends on what the others say. A container's name is noise when
// every change is inside the same one, and the only way to tell two lines
// apart when they are not.
func shortLines(changes []Change) []string {
	var (
		at     = make([]location, len(changes))
		groups = map[string]bool{}
	)

	for i, c := range changes {
		at[i] = locate(c)

		if at[i].group != "" {
			groups[at[i].group] = true
		}
	}

	var (
		scopes   = make([]string, len(changes))
		distinct = map[string]bool{}
		width    int
	)

	for i, l := range at {
		scopes[i] = l.scope(len(groups) > 1)
		distinct[scopes[i]] = true
		width = max(width, len(scopes[i]))
	}

	out := make([]string, 0, len(changes))

	for i, c := range changes {
		// One scope over the whole set says nothing any line needs: it is the
		// same answer for every one of them, and the column would push every
		// change to the right to repeat it.
		if len(distinct) == 1 {
			out = append(out, describeAs(c, at[i].field))

			continue
		}

		out = append(out, fmt.Sprintf("%-*s  %s", width, scopes[i], describeAs(c, at[i].field)))
	}

	return out
}
