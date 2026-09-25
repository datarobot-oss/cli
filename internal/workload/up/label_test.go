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
	"testing"

	"github.com/stretchr/testify/assert"
)

// The reported line: a variable's value moved, and the name of it was at the
// end of a path long enough to wrap on an ordinary terminal, which put the one
// word the reader wanted on a line of its own.
func TestShortLines_DropsTheScaffoldingAboveTheField(t *testing.T) {
	changed := moved(inEnv("echo", "SE_MOVED", "value"), "a", "b")

	assert.Equal(t, []string{"env SE_MOVED: changed"}, shortLines([]Change{changed}))
}

// The container's name is dropped because every change shares it, not because
// it does not matter: two containers in one plan get a column, or the reader
// cannot tell which of them the line is about.
func TestShortLines_NamesTheContainerWhenTheChangesSpanMoreThanOne(t *testing.T) {
	lines := shortLines([]Change{
		moved(inEnv("echo", "SE_MOVED", "value"), "a", "b"),
		moved(inContainer("sidecar", "port"), 8080.0, 9090.0),
	})

	assert.Equal(t, []string{
		"echo     env SE_MOVED: changed",
		"sidecar  port: 8080 -> 9090",
	}, lines)
}

// A group and a container inside it are two different places to change
// something, and the group's own fields are what the runtime applies in place.
func TestShortLines_TellsAGroupApartFromItsContainers(t *testing.T) {
	lines := shortLines([]Change{
		moved(inGroup("default", "replicaCount"), 1.0, 3.0),
		moved(inContainer("echo", "port"), 8080.0, 9090.0),
	})

	assert.Equal(t, []string{
		"default  replicaCount: 1 -> 3",
		"echo     port: 8080 -> 9090",
	}, lines)
}

// Two groups can hold containers of the same name, so the container alone
// stops being an answer and the group joins it.
func TestShortLines_AddsTheGroupWhenTheChangesSpanMoreThanOne(t *testing.T) {
	lines := shortLines([]Change{
		moved(inGroupContainer("cpu", "worker", "port"), 8080.0, 9090.0),
		moved(inGroupContainer("gpu", "worker", "port"), 8080.0, 9091.0),
	})

	assert.Equal(t, []string{
		"cpu/worker  port: 8080 -> 9090",
		"gpu/worker  port: 8080 -> 9091",
	}, lines)
}

// A field outside any group keeps its own name and sits in an empty column,
// which is what says it is not inside one.
func TestShortLines_LeavesAFieldOutsideAnyGroupAlone(t *testing.T) {
	lines := shortLines([]Change{
		moved(Change{Path: keyArtifactID, Keys: []string{keyArtifactID}}, "68a", "68b"),
		moved(inContainer("echo", "port"), 8080.0, 9090.0),
	})

	assert.Equal(t, []string{
		"      artifactId: 68a -> 68b",
		"echo  port: 8080 -> 9090",
	}, lines)
}

// A whole container the live workload does not carry has no field under it to
// name, so its own name is the line. Dropping it into the column would be the
// one case where the scope is the whole fact and the scope is what gets
// dropped when the changes share it.
func TestShortLines_NamesAWholeElementThatIsMissing(t *testing.T) {
	container := absent(inContainer("sidecar"))
	container.Want = map[string]any{"name": "sidecar", "imageUri": "a:1"}

	assert.Equal(t, []string{"container sidecar: {imageUri, name}"}, shortLines([]Change{container}))
}

// A container whose name holds a bracket renders a path that cannot be parsed
// back into the names that built it, which is why the label is built from the
// keys the walk descended through.
func TestShortLines_ReadsTheKeysRatherThanThePath(t *testing.T) {
	bracketed := moved(Change{
		Path: "containerGroups[default].containers[a]b].imageUri",
		Keys: []string{keyContainerGroups, "default", keyContainers, "a]b", "imageUri"},
	}, "a:1", "a:2")

	assert.Equal(t, []string{"imageUri: a:1 -> a:2"}, shortLines([]Change{bracketed}))
}

// A change built without keys is one this cannot take apart, so it keeps the
// whole path rather than being guessed at.
func TestShortLines_KeepsThePathOfAChangeWithoutKeys(t *testing.T) {
	bare := moved(Change{Path: "containerGroups[default].containers[echo].port"}, 8080.0, 9090.0)

	assert.Equal(t, []string{"containerGroups[default].containers[echo].port: 8080 -> 9090"},
		shortLines([]Change{bare}))
}

// inGroupContainer is inContainer with the group named too, for the plans that
// span more than the one every other fixture uses.
func inGroupContainer(group, name string, field ...string) Change {
	c := inContainer(name, field...)
	c.Keys[1] = group
	c.Path = "containerGroups[" + group + "]" + c.Path[len("containerGroups[default]"):]

	return c
}

// Two groups gaining a container of the same name. The element's own name goes
// in the field, but the group it is being added to is the only thing that
// tells the two lines apart, so dropping it rendered them identically.
func TestShortLines_TellsTwoGroupsGainingTheSameNameApart(t *testing.T) {
	blue := absent(inGroupContainer("blue", "primary"))
	blue.Want = map[string]any{"name": "primary", "imageUri": "a:1"}

	green := absent(inGroupContainer("green", "primary"))
	green.Want = map[string]any{"name": "primary", "imageUri": "b:1"}

	assert.Equal(t, []string{
		"blue   container primary: {imageUri, name}",
		"green  container primary: {imageUri, name}",
	}, shortLines([]Change{blue, green}))
}

// The milder half of the same fault: a whole missing container beside a change
// inside another group used to sit under a blank scope, because the location it
// was reduced to carried no group for the column to print.
func TestShortLines_KeepsTheGroupOfAMissingContainer(t *testing.T) {
	missing := absent(inGroupContainer("blue", "primary"))
	missing.Want = map[string]any{"name": "primary", "imageUri": "a:1"}

	assert.Equal(t, []string{
		"blue           container primary: {imageUri, name}",
		"green/sidecar  port: 8080 -> 9090",
	}, shortLines([]Change{missing, moved(inGroupContainer("green", "sidecar", "port"), 8080.0, 9090.0)}))
}
