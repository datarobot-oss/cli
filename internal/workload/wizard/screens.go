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
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// createNewID is the sentinel the "create a new workload" row carries, so the
// picker can return it through the same channel as a real workload.
const createNewID = "\x00create-new"

// defaultEditor is what [e] opens when nothing names an editor. It matches the
// default the root command registers for the same setting, so the wizard opens
// the same thing whether or not that registration has run.
const defaultEditor = "vi"

// healthPlaceholder says what an empty health path means rather than showing
// the default, which is what every other placeholder on that screen does.
// Ghosting the default path there would advertise the opposite of what
// accepting the empty field does, and the field is only ever empty when a probe
// has been declined or the bound workload runs without one.
const healthPlaceholder = "none: no readiness probe"

// namePlaceholder shows the shape of a workload name without proposing one.
// The directory is a poor suggestion often enough (src, app, cli, tmp) that
// offering it invites a name nobody chose, and this name ends up on a
// deployed workload.
const namePlaceholder = "my-app-name"

// runInteractiveFlow asks the questions and returns the manifest the user
// agreed to. The workload list is fetched up front because the first screen
// depends on it: with nothing to bind to, there is no binding question.
func runInteractiveFlow(opts Options, detected Detected) ([]byte, manifest.Draft, error) {
	// No screen can settle these, so they are not questions the wizard gets to
	// ask. Started anyway, the error would sit behind the loading view while
	// the fetch ran, and arriving at the first screen would clear it and drop
	// the flag without saying so.
	//
	// Only those two. A bad --port, a path missing its slash and an importance
	// outside the enum are all values a screen shows and the user can correct,
	// and refusing them here would turn a recoverable typo into a run that
	// never starts.
	for _, check := range []func() error{opts.Answers.checkBinding, opts.Answers.checkProbeExclusive} {
		if err := check(); err != nil {
			return nil, manifest.Draft{}, err
		}
	}

	var workloads []workload.Workload

	if opts.Answers.WorkloadID == "" && opts.Answers.Name == "" {
		fetched, err := listWorkloadsFn(workloadPickLimit, nil, "")
		if err != nil {
			return nil, manifest.Draft{}, err
		}

		workloads = fetched
	}

	// The wizard draws on stderr, not stdout. stdout is this command's
	// machine-readable channel, and bubbletea would otherwise send the alt
	// screen and every redraw there, so `dr workload config` inside a command
	// substitution would capture escape sequences ahead of the path.
	//
	started := newFlow(detected, workloads, opts.Answers)
	// Carried into the flow because the confirm screen is where secrets would
	// be stored, and --dry-run has to mean the same thing on a terminal as it
	// does headless: nothing created, here or on the platform.
	started.dryRun = opts.DryRun

	// tui.Run already wraps the model for Ctrl-C, so it is not wrapped here.
	final, err := tui.Run(started,
		tea.WithAltScreen(), tea.WithOutput(interactiveOutput(opts.Stderr)))
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	finished, ok := tui.Unwrap(final).(flow)
	if !ok {
		return nil, manifest.Draft{}, ErrCancelled
	}

	// Printed here rather than from inside the flow: until tui.Run returns,
	// the alt screen owns the terminal and anything written to stderr lands
	// underneath a full-screen redraw.
	reportImport(opts.Stderr, finished.imports)

	return finished.result()
}

// interactiveOutput is where the wizard draws. bubbletea needs the real
// *os.File to size the terminal and restore raw mode, so a plain io.Writer
// (a test buffer) falls back to os.Stderr rather than being drawn into.
func interactiveOutput(stderr io.Writer) io.Writer {
	if file, ok := stderr.(*os.File); ok {
		return file
	}

	return os.Stderr
}

// enter prepares the components a screen needs. It runs on the way forward
// and on the way back, so returning to a screen shows the answer that was
// given there rather than a blank field.
func (f *flow) enter(at screen) {
	f.failed = nil
	f.focus = 0

	switch at {
	case screenBinding, screenExecEnv:
		f.enterPicker(at)

	case screenKind, screenA2A, screenSource:
		f.enterChoice(at)
		f.choice.liveValue = f.liveChoice(at)

	case screenName, screenEntrypoint, screenImage, screenSettings:
		f.enterInputs(at)

	case screenEnv:
		// Built once. Coming back to the screen has to show the answers given
		// there, not the classifier's opening proposal all over again.
		if len(f.envTable.rows) == 0 {
			f.envTable = newEnvTable(f.detected.EnvVars, f.width, f.height)
		}

	case screenConfirm:
		// Rendered from the answers already given.
	}
}

// enterPicker rebuilds a list screen and puts the cursor back on the answer
// given there, so re-confirming does not silently take whatever is first.
func (f *flow) enterPicker(at screen) {
	switch at {
	case screenBinding:
		f.picker = newWorkloadPicker(f.workloads, f.width, f.height)

		if id := f.draft.WorkloadID; id != "" {
			f.picker.selectValue(func(v any) bool {
				picked, ok := v.(pickedWorkload)

				return ok && picked.id == id
			})
		}

	case screenExecEnv:
		// Rebuilt from what was already fetched, so walking back here shows
		// the list again. On a first visit there is nothing yet, and emptying
		// the shared table means a failed load cannot leave the previous
		// screen's workload rows on display under a base-image heading.
		f.picker = newExecEnvPicker(f.execEnvs, f.width, f.height)

		if id := f.draft.Build.ExecutionEnvironmentID; id != "" {
			f.picker.selectValue(func(v any) bool {
				picked, ok := v.(pickedEnv)

				return ok && picked.id == id
			})
		}

	case screenName, screenKind, screenA2A, screenSource, screenEntrypoint,
		screenImage, screenSettings, screenEnv, screenConfirm:
		// Not lists.
	}
}

// enterChoice prepares the menu screens.
func (f *flow) enterChoice(at screen) {
	switch at {
	case screenKind:
		f.choice = newChoice([]option{
			{value: manifest.TypeService, label: "Service", note: "standard HTTP workload"},
			{value: manifest.TypeAgent, label: "Agent", note: "HTTP workload with agent capabilities"},
		}, f.draft.Type, "Keep this workload's kind")

	case screenA2A:
		f.choice = newChoice([]option{
			{value: "no", label: "No", note: "reachable only at its own endpoint"},
			{value: "yes", label: "Yes", note: "discoverable tenant-wide"},
		}, yesNo(f.draft.A2AEnabled), "")

	case screenSource:
		f.choice = newChoice(f.sourceOptions(), f.draft.Build.Mode, "Keep how it is built today")

	case screenBinding, screenExecEnv, screenEnv, screenName, screenEntrypoint, screenImage, screenSettings, screenConfirm:
		// Not menus.
	}
}

// enterInputs prepares the text-entry screens.
func (f *flow) enterInputs(at screen) {
	switch at {
	case screenName:
		// The suggested name is a placeholder rather than a value, so
		// accepting it is one Enter and replacing it does not start with
		// clearing a field. A name the user actually gave is a value, and
		// comes back as one.
		if f.nameGiven {
			f.inputs = []textinput.Model{newInput(f.draft.Name, namePlaceholder)}
		} else {
			f.inputs = []textinput.Model{newInput("", namePlaceholder)}
		}

	case screenEntrypoint:
		f.inputs = []textinput.Model{newInput(strings.Join(f.draft.Build.Entrypoint, " "), "python main.py")}

	case screenImage:
		f.inputs = []textinput.Model{newInput(f.draft.Build.ImageURI, "registry.example.com/team/app:v1")}

	case screenSettings:
		inputs := make([]textinput.Model, settingsFieldCount)
		inputs[fieldPort] = newInput(strconv.Itoa(f.draft.Port), "8080")
		inputs[fieldHealthPath] = newInput(f.draft.HealthPath, healthPlaceholder)
		inputs[fieldReplicas] = newInput(replicaValue(f.draft.Runtime.Replicas), replicaPlaceholder(f.autoscaled()))
		inputs[fieldCPU] = newInput(formatCPU(f.draft.Runtime.CPU), "0.5")
		inputs[fieldMemory] = newInput(f.draft.Runtime.Memory, manifest.DefaultMemory)
		inputs[fieldImportance] = newInput(f.draft.Importance, manifest.DefaultImportance)

		f.inputs = inputs

		// newInput focuses everything it builds, so exactly one is left
		// focused here. Through applyFocus rather than a hardcoded index, so
		// arrival and movement agree on where the cursor is by construction.
		// The blink command is parked for onEnter to hand on: arriving at a
		// screen has no tea.Cmd of its own to return it through.
		f.focusCmd = f.applyFocus()

	case screenBinding, screenExecEnv, screenKind, screenA2A, screenSource, screenEnv, screenConfirm:
		// No text entry.
	}
}

// sourceOptions annotates the image sources with what the project shows, so
// the recommended answer is the one the evidence points at rather than the
// one we happen to prefer.
func (f flow) sourceOptions() []option {
	// The absent case is a warning, not a detail: choosing it fails, and the
	// screen refuses it rather than letting the build discover it later.
	dockerfileNote, missing := DockerfileName+" detected", false
	if !f.detected.HasDockerfile {
		dockerfileNote, missing = "no "+DockerfileName+" detected", true
	}

	return []option{
		{
			value: manifest.BuildModeDockerfile,
			label: "Build this repository's " + DockerfileName,
			note:  dockerfileNote,
			warn:  missing,
		},
		{value: manifest.BuildModeGenerated, label: "Build from a managed execution environment"},
		{value: manifest.BuildModeImage, label: "Use an already published image"},
	}
}

// View renders the current screen: a question, the answer being given, and
// the keys that move.
// View lays every screen out the same way: the question, what is being
// answered, the note explaining it, anything that went wrong, and the keys.
// Each block is separated by a blank line, because a wizard where the
// explanation touches the table reads as one wall of text.
func (f flow) View() string {
	if f.loading != "" {
		return "\n" + questionStyle.Render(f.loading) + "\n"
	}

	// Two kinds of prose, and they belong in different places. The intro
	// explains the question, so it is read before the answer. The note is
	// about the answer being given right now -- the row under the cursor,
	// what the bound workload already uses -- so it belongs after it.
	blocks := []string{
		questionStyle.Render(f.question()),
		f.note(f.screenIntro()),
		strings.TrimRight(f.body(), "\n"),
		f.note(f.screenNote()),
	}

	if f.failed != nil {
		blocks = append(blocks,
			tui.ErrorStyle.MarginLeft(2).Width(contentWidth(f.width)).Render(f.failed.Error()))
	}

	blocks = append(blocks, f.keys())

	present := make([]string, 0, len(blocks))

	for _, block := range blocks {
		if strings.TrimSpace(block) != "" {
			present = append(present, block)
		}
	}

	return "\n" + strings.Join(present, "\n\n") + "\n"
}

// liveChoice is the option the bound workload already uses on this screen,
// "" on a fresh setup or a screen with no live counterpart.
func (f flow) liveChoice(at screen) string {
	if f.live == nil {
		return ""
	}

	live := f.live.Defaults()

	switch at {
	case screenKind:
		return live.Type
	case screenA2A:
		return yesNo(live.A2AEnabled)
	case screenSource:
		return live.Build.Mode
	case screenBinding, screenName, screenExecEnv, screenEntrypoint,
		screenImage, screenSettings, screenEnv, screenConfirm:
		return ""
	}

	return ""
}

// liveDefaultsNote ties the "in use now" tag to what it means, so a bound run
// is not mistaken for a fresh one and the preselected row is not mistaken for
// the wizard's own guess.
func (f flow) liveDefaultsNote() string {
	if f.live == nil {
		return ""
	}

	return liveStyle.Render("· in use") + " marks " + f.live.Name +
		"'s current configuration. Changing it changes the workload."
}

// liveValuesNote is the same idea for the screens made of fields rather than
// options, where there is no row to tag.
func (f flow) liveValuesNote() string {
	if f.live == nil {
		return ""
	}

	values := "These are " + liveStyle.Render(f.live.Name) +
		"'s current values. Changing one changes the workload."

	// On the settings screen most of those values are behind the advanced row,
	// so the sentence would otherwise promise a review of things that are not
	// on screen.
	if f.at == screenSettings && !f.advancedOpen {
		return values + " The rest are under advanced options."
	}

	return values
}

// sourceConflictNote warns when the workload builds a Dockerfile that this
// checkout does not have. Preselecting that option would otherwise look like
// a working default right up until the screen refuses to accept it.
func (f flow) sourceConflictNote() string {
	if f.live == nil || f.detected.HasDockerfile {
		return ""
	}

	if f.live.Defaults().Build.Mode != manifest.BuildModeDockerfile {
		return ""
	}

	return tui.ErrorStyle.Render("No "+DockerfileName+" in this directory,") +
		" so this repository cannot build the way " + f.live.Name + " currently does. Add one, or select another source."
}

// screenIntro explains the question itself: the same words every time the
// screen is shown, read before anything is answered.
func (f flow) screenIntro() string {
	switch f.at {
	case screenName:
		return ""
	case screenKind:
		return "Both run as containers and are built the same way. Agent additionally enables " +
			"agent capabilities, the first of which is A2A discovery."
	case screenA2A:
		return "Publishing reads the A2A agent card from the running container and registers it " +
			"tenant-wide, so other agents can discover and call this one. The application must " +
			"serve a card for this to work."
	case screenEntrypoint:
		return "The container entrypoint: the command run on start, written to `entrypoint` in the manifest."
	case screenEnv:
		return "Variables from " + EnvFileName + ". Secrets become credential references; " +
			"everything else is written as a literal."
	case screenBinding, screenSource, screenExecEnv, screenImage, screenSettings, screenConfirm:
		return ""
	}

	return ""
}

// screenNote is about the answer being given right now: the row under the
// cursor, the field being edited, what the bound workload already uses.
func (f flow) screenNote() string {
	switch f.at {
	case screenBinding:
		return f.bindingNote()
	case screenSettings:
		return joinNotes(f.liveValuesNote(), f.settingsNote())
	case screenEnv:
		return f.envTable.note()
	case screenConfirm:
		return f.confirmNote()
	case screenKind, screenSource, screenImage, screenExecEnv:
		return f.boundScreenNote()
	case screenName, screenA2A, screenEntrypoint:
		return ""
	}

	return ""
}

// boundScreenNote covers the screens whose only note is about the workload
// being bound to: which option it already uses, and whether this checkout can
// honour it.
func (f flow) boundScreenNote() string {
	switch f.at {
	case screenKind:
		return f.liveDefaultsNote()
	case screenSource:
		// The Dockerfile warning belongs to this screen alone: it is about
		// the option being chosen here, not about the workload in general.
		return joinNotes(f.liveDefaultsNote(), f.sourceConflictNote())
	case screenImage:
		return f.liveValuesNote()
	case screenBinding, screenName, screenA2A, screenExecEnv,
		screenEntrypoint, screenSettings, screenEnv, screenConfirm:
		return ""
	}

	return ""
}

// questions is what each screen asks. Phrasing them in one place keeps the
// wizard sounding like one voice rather than ten.
var questions = map[screen]string{
	screenBinding:    "Which workload should this repository deploy to?",
	screenName:       "Name of your workload",
	screenKind:       "What kind of workload is this?",
	screenA2A:        "Publish this agent to the A2A registry?",
	screenSource:     "How should the container image be produced?",
	screenExecEnv:    "Which execution environment should the build use?",
	screenEntrypoint: "What is your app's entrypoint?",
	screenImage:      "Which image should it run?",
	screenSettings:   "Which port does your app listen on?",
	screenEnv:        "Which variables should the manifest declare?",
	screenConfirm:    "Ready to write " + manifest.FileName,
}

func (f flow) question() string {
	// The settings screen asks about the port until the advanced row is
	// opened, at which point five more fields are on screen and naming one of
	// them stops describing what is being answered.
	if f.at == screenSettings && f.advancedOpen {
		return "How should this workload run?"
	}

	return questions[f.at]
}

func (f flow) body() string {
	switch f.at {
	case screenBinding, screenExecEnv:
		return f.picker.view(f.width)

	case screenKind, screenSource:
		return f.choice.view()

	case screenEnv:
		return f.envTable.view(f.width)

	case screenA2A:
		return f.choice.view()

	case screenName, screenEntrypoint, screenImage:
		return "  " + f.inputs[0].View()

	case screenSettings:
		return f.settingsView()

	case screenConfirm:
		return f.confirmBody()
	}

	return ""
}

// The settings screen's fields, in the order they are asked, which is the
// order the file reads: how the container is reached, then how much of it
// runs.
//
// Four things are keyed by this order — the inputs enterInputs builds, the
// labels beside them, the notes under them, and the reads in acceptSettings —
// so the position is named here once instead of written as a number in each of
// them. A number that falls out of step with the others reads the wrong field
// or panics, and neither announces itself.
const (
	fieldPort = iota
	fieldHealthPath
	fieldReplicas
	fieldCPU
	fieldMemory
	fieldImportance

	// settingsFieldCount sizes everything indexed by the above, so an entry
	// added past the end is a compile error rather than a silent no-op.
	settingsFieldCount
)

// The two positions that are not fields. They live outside the block above
// because a constant with an explicit value ends its iota run: written there,
// the next field appended with no value of its own would silently repeat -1
// instead of continuing the count, which is the opposite of what that block
// promises.
const (
	// firstAdvancedField is where the shown-by-default part of the screen
	// ends. Everything from here on is behind the advanced row, which is why
	// the field order puts the port first: the advanced set is a suffix of one
	// list rather than a second list that could fall out of step with it.
	firstAdvancedField = fieldHealthPath

	// advancedStop is where the cursor sits when the advanced row itself is
	// focused. It is not a field, so it indexes nothing; every place that
	// reads f.focus as a field has to allow for it.
	advancedStop = -1
)

// settingsLabels name the fields. Everything here has a working default, so
// the screen is a review as much as a question.
var settingsLabels = [settingsFieldCount]string{
	fieldPort:       "Port",
	fieldHealthPath: "Health path",
	fieldReplicas:   "Replicas",
	fieldCPU:        "CPU",
	fieldMemory:     "Memory",
	fieldImportance: "Importance",
}

// settingsNotes explain the field the cursor is on, one at a time. They sit
// under the fields rather than beside them: an inline hint on the widest row
// runs off a narrow terminal, and only the field being edited needs
// explaining anyway.
var settingsNotes = [settingsFieldCount]string{
	fieldPort:       "Port the container listens on. Must be 1024 or above: containers run unprivileged.",
	fieldHealthPath: "Readiness probe path. While it is set, traffic is withheld until this endpoint reports success. Leave it empty to run without a probe.",
	fieldReplicas:   "Container instances to run. Reported as protons in status output and logs.",
	fieldCPU:        "CPU cores per replica. Fractional values are allowed.",
	fieldMemory:     "Memory per replica. A byte count or a 1000-based unit (" + strings.Join(manifest.MemoryUnits(), ", ") + "). Binary units such as Gi are rejected: they would be read as their decimal equivalents.",
	fieldImportance: "Scheduling priority when capacity is constrained: " + strings.Join(manifest.ImportanceLevels, ", ") + ".",
}

// settingsView shows the one field that decides whether the container is
// reachable, and offers the rest behind a row that opens.
//
// The hidden fields all have a working default and are all settable by flag,
// and the confirm screen states every one of them before anything is written,
// so a user who never opens the row still sees what they are agreeing to.
func (f flow) settingsView() string {
	var b strings.Builder

	fmt.Fprintf(&b, "  %s  %s\n\n",
		labelStyle.Render(settingsLabels[fieldPort]), f.inputs[fieldPort].View())
	fmt.Fprintf(&b, "  %s\n", f.advancedRow())

	if !f.advancedOpen {
		return strings.TrimRight(b.String(), "\n")
	}

	width := 0
	for field := firstAdvancedField; field < settingsFieldCount; field++ {
		width = max(width, len(settingsLabels[field]))
	}

	// Indented under the row that revealed them, so the screen reads as one
	// field and a drawer rather than as six fields again.
	for field := firstAdvancedField; field < settingsFieldCount; field++ {
		padded := settingsLabels[field] + strings.Repeat(" ", width-len(settingsLabels[field]))
		fmt.Fprintf(&b, "    %s  %s\n", labelStyle.Render(padded), f.inputs[field].View())
	}

	return strings.TrimRight(b.String(), "\n")
}

// advancedRow is the control that shows and hides the rest of the screen. The
// arrow points the way pressing it goes, and the row highlights like any other
// answer under the cursor, because that is what it is.
func (f flow) advancedRow() string {
	label := "▸ Advanced options"
	if f.advancedOpen {
		label = "▾ Advanced options"
	}

	if f.focus == advancedStop {
		return selectedStyle.Render(label)
	}

	return labelStyle.Render(label)
}

// onAdvancedRow is the one place that says what the advanced row being focused
// looks like, so the sentinel is read the same way everywhere rather than
// spelled out with and without the screen check depending on the call site.
func (f flow) onAdvancedRow() bool {
	return f.at == screenSettings && f.focus == advancedStop
}

// advancedKey opens and closes the advanced row from anywhere on the settings
// screen, so the fields behind it are reachable without first discovering that
// tab lands on a row.
//
// A chord rather than a letter, because every stop on this screen but the row
// itself is a text input and a bare key is text there. ctrl+a would have been
// the mnemonic; the input already binds it to line-start, along with b, d, e,
// f, h, k, n, p, u, v and w, and taking a standard editing key away from a
// field to save one keystroke elsewhere is a poor trade.
const advancedKey = "ctrl+o"

// advancedLabel is what both keys that toggle the row are called in the
// legend. One word in either state, because the row's own arrow already says
// which way it will go and a label that changes width wraps the legend onto a
// second line at eighty columns, pushing [esc] back off the end of the first.
const advancedLabel = "advanced"

// settingsNote explains whatever the cursor is on. The port's note carries its
// provenance too, since that is the one value detection can vouch for.
func (f flow) settingsNote() string {
	if f.onAdvancedRow() {
		return "Readiness probe, sizing and scheduling. Every one has a working default, " +
			"and the confirm screen shows what will be written whether or not you open this."
	}

	if f.focus < 0 || f.focus >= len(settingsNotes) {
		return ""
	}

	note := settingsNotes[f.focus]

	switch f.focus {
	case fieldPort:
		// Provenance is about what this checkout showed. A bound workload's
		// port came from the workload, so quoting the Dockerfile there would
		// credit a source the value on screen did not come from.
		if f.live == nil {
			note += " " + capitalize(f.detected.PortSource) + "."
		}
	case fieldHealthPath:
		note = joinNotes(note, f.liveProbeNote())
	}

	return note
}

// liveProbeNote explains an empty field on a bound workload, which has two
// quite different causes. The workload may genuinely run without a probe, in
// which case the empty field is its own configuration rather than something
// the wizard failed to fill in. Or it may run a probe shaped in a way this
// release cannot read, in which case the field is not about that probe at all
// and nothing typed here will touch it.
func (f flow) liveProbeNote() string {
	if f.live == nil {
		return ""
	}

	present, readable := f.liveReadinessProbe()

	switch {
	case readable:
		return ""

	case present:
		return liveStyle.Render(f.live.Name) + " runs a readiness probe with no path, so this field does not " +
			"apply to it. Its shape is kept as it is; only its port follows the container's, the way the " +
			"startup and liveness probes do."

	default:
		return liveStyle.Render(f.live.Name) + " runs without a readiness probe. Leaving this empty keeps it that way; " +
			"a path adds one, and the container has to serve it."
	}
}

// capitalize raises the first letter so a detection phrase reads as a
// sentence where it is appended to one.
func capitalize(text string) string {
	if text == "" {
		return ""
	}

	return strings.ToUpper(text[:1]) + text[1:]
}

// formatCPU renders a CPU allocation the way it would be typed, so a whole
// number does not come back as 1.0.
func formatCPU(cpu float64) string {
	return strconv.FormatFloat(cpu, 'g', -1, 64)
}

// joinNotes stacks notes that both apply, dropping the ones that do not.
func joinNotes(notes ...string) string {
	present := make([]string, 0, len(notes))

	for _, note := range notes {
		if note != "" {
			present = append(present, note)
		}
	}

	return strings.Join(present, "\n")
}

// note renders an explanatory sentence, wrapped to the terminal.
func (f flow) note(text string) string {
	if text == "" {
		return ""
	}

	return noteStyle.Width(contentWidth(f.width)).Render(text)
}

// bindingNote explains the row under the cursor: what picking it does.
func (f flow) bindingNote() string {
	row := f.picker.selected()
	if row == nil {
		return ""
	}

	item, ok := row.value.(pickedWorkload)
	if !ok {
		return ""
	}

	if item.id == createNewID {
		return "Creates a new workload for this repository. You name it on the next screen."
	}

	return "This repository will deploy to " + item.name + ": the same workload, not a copy. Its " +
		"current configuration is read into the manifest first, so nothing it runs with is dropped, " +
		"and the exact changes are shown before anything is written."
}

// confirmBody shows the file, and for a bound workload what it changes about
// something already running.
func (f flow) confirmBody() string {
	if f.failed != nil {
		return ""
	}

	return indent(f.preview.View())
}

// previewText is what the confirm screen is asking about: the file itself, or
// for a bound workload the change it makes to something already running.
func (f flow) previewText() string {
	if f.diff != "" {
		return f.diff
	}

	if f.live != nil {
		return string(f.content) + "\n" + tui.HintStyle.Render("no change to the running workload")
	}

	return string(f.content)
}

// buildPreview fits the preview to the terminal. It runs wherever the content
// is settled rather than at screen entry, because the render happens after
// enter and the editor can replace the file afterwards.
func (f *flow) buildPreview() {
	text := strings.TrimRight(f.previewText(), "\n")
	lines := strings.Count(text, "\n") + 1

	// A short manifest keeps its own height: padding it out to a fixed window
	// would put the key legend below the fold for no reason.
	f.preview = viewport.New(contentWidth(f.width), min(f.previewHeight(), lines))
	f.preview.SetContent(text)
}

// scrollablePreview reports whether the confirm screen has more than it can
// show, which decides whether the legend offers a key for it.
func (f flow) scrollablePreview() bool {
	return f.preview.TotalLineCount() > f.preview.Height
}

// How much of the manifest the confirm screen shows at once: everything the
// terminal has left once the question, the note under it and the key legend
// are accounted for. There is no upper cap, because the whole point of the
// screen is to read the file, and none below the point where scrolling is all
// there is to see.
const (
	defaultPreviewHeight = 20
	minPreviewHeight     = 6
	// What the screen spends on everything that is not the note: the question,
	// the key legend and the blank lines between the blocks. The note is
	// measured rather than budgeted for, because its height moves with the
	// terminal's width and with how much there is to say.
	previewChrome = 9
)

// previewHeight is what the terminal has left for the file itself. Measuring
// the note beats reserving a constant for it: a fixed budget large enough for
// a wrapped four-line note steals a line from every screen that has three, and
// one tuned to three pushes the key legend off the bottom on a narrow terminal.
func (f flow) previewHeight() int {
	if f.height <= 0 {
		return defaultPreviewHeight
	}

	return max(f.height-previewChrome-lipgloss.Height(f.note(f.confirmNote())), minPreviewHeight)
}

// confirmNote is what the user is agreeing to, beyond the file itself: where
// it goes, how much it will run with, and what happens next.
func (f flow) confirmNote() string {
	if f.failed != nil {
		return ""
	}

	// joinNotes rather than strings.Join: readinessLine says nothing about a
	// hand-edited file, and an empty entry would leave a blank line mid-note.
	return joinNotes(
		"Writing to "+ShortPath(manifest.Path(f.detected.Dir)),
		f.runtimeLine(),
		f.readinessLine(),
		f.nextStep(),
	)
}

// runtimeLine summarises the sizing and scheduling the file will carry. A
// bound workload reports its own, not the documented defaults, because this is
// the line the user is consenting to. The settings screen keeps all of it
// behind the advanced row, so this line is where most users see it at all.
func (f flow) runtimeLine() string {
	sizing := fmt.Sprintf("%s cpu · %s",
		strconv.FormatFloat(f.draft.Runtime.CPU, 'g', -1, 64), f.draft.Runtime.Memory)

	replicas := "autoscaling"
	if f.draft.Runtime.Replicas > 0 {
		replicas = fmt.Sprintf("%d replica", f.draft.Runtime.Replicas)
		if f.draft.Runtime.Replicas != 1 {
			replicas += "s"
		}
	}

	importance := orDefault(f.draft.Importance, manifest.DefaultImportance)

	return "Runtime: " + replicas + " · " + sizing + " · importance " + importance +
		"    (edit the file for GPU/autoscaling)"
}

// readinessLine says what the file will carry for the probe, the other value
// the advanced row hides. An absent probe is stated rather than left out, so
// the two cases cannot be told apart only by what is missing.
//
// It describes the answers, so it says nothing once the file has been edited
// by hand: the bytes on screen are then the truth, and a summary derived from
// the draft would contradict the preview directly above it.
func (f flow) readinessLine() string {
	if f.handEdited {
		return ""
	}

	// A probe shape this release does not model is carried over untouched, so
	// the answers do not describe it and neither should this line.
	if present, readable := f.liveReadinessProbe(); present && !readable {
		return "Readiness: " + f.live.Name + "'s own probe, kept as it is apart from the port it watches."
	}

	if !f.draft.WantsReadinessProbe() {
		return "Readiness: no probe, so traffic is not held back for a health check."
	}

	return fmt.Sprintf("Readiness: %s on port %d", f.draft.HealthPath, f.draft.Port)
}

// liveReadinessProbe is what the bound workload does about readiness, and
// false for an unbound run, so callers can ask without repeating the nil check.
func (f flow) liveReadinessProbe() (present, readable bool) {
	if f.live == nil {
		return false, false
	}

	return f.live.ReadinessProbe()
}

func (f flow) nextStep() string {
	if f.live != nil {
		return "On `up`: deploys this repo to " + f.live.Name + "."
	}

	return "On `up`: creates a new workload and deploys this repo to it."
}

// screenKeys is the footer each screen shows. Screens not listed take the
// default: answer and move, or go back.
var screenKeys = map[screen][]keyBinding{
	screenConfirm: {
		{Key: "enter", Does: "write the file"},
		{Key: "e", Does: "open in your editor"},
		{Key: "esc", Does: "back"},
	},
	screenBinding: {
		{Key: "type", Does: "to filter"},
		{Key: "↑/↓", Does: "move"},
		{Key: "enter", Does: "select"},
		{Key: "esc", Does: "back"},
	},
	screenExecEnv: {
		{Key: "type", Does: "to filter"},
		{Key: "↑/↓", Does: "move"},
		{Key: "enter", Does: "select"},
		{Key: "esc", Does: "back"},
	},
	screenSettings: {
		{Key: "tab", Does: "next field"},
		{Key: advancedKey, Does: "advanced"},
		{Key: "enter", Does: "continue"},
		{Key: "esc", Does: "back"},
	},
	screenEnv: {
		{Key: "↑/↓", Does: "move"},
		{Key: "←/→", Does: "literal · credential · omit"},
		{Key: "enter", Does: "accept"},
		{Key: "esc", Does: "back"},
	},
}

var defaultKeys = []keyBinding{
	{Key: "enter", Does: "continue"},
	{Key: "esc", Does: "back"},
}

// withKey re-labels one key in a screen's legend, copying first because
// screenKeys is a package-level map and the slices in it are shared by every
// render. A key the screen does not list is left alone: every caller relabels a
// binding the screen already has, and appending one instead would put a key in
// the legend that nothing on that screen answers.
func withKey(bindings []keyBinding, key, does string) []keyBinding {
	out := slices.Clone(bindings)

	for i := range out {
		if out[i].Key == key {
			out[i].Does = does

			break
		}
	}

	return out
}

// withoutKey drops a binding, for the states where a key the screen normally
// offers is not the way to do the thing any more.
func withoutKey(bindings []keyBinding, key string) []keyBinding {
	return slices.DeleteFunc(slices.Clone(bindings), func(b keyBinding) bool {
		return b.Key == key
	})
}

func (f flow) keys() string {
	bindings, ok := screenKeys[f.at]
	if !ok {
		bindings = defaultKeys
	}

	// Offered only when there is something below the fold, so the legend
	// never advertises a key that does nothing.
	if f.at == screenConfirm && f.scrollablePreview() {
		bindings = append([]keyBinding{{Key: "↑/↓", Does: "scroll"}}, bindings...)
	}

	// On the row itself enter is the row's own action, so it is labelled for
	// that and the chord drops out: two entries reading "advanced" say the
	// screen has two ways to do one thing, in the one place that is not worth
	// the width. Derived from the table rather than rewritten, so a key the
	// screen gains later still appears here.
	if f.onAdvancedRow() {
		bindings = withoutKey(withKey(bindings, "enter", advancedLabel), advancedKey)
	}

	// With a filter in place, esc undoes that first. Saying "back" would be a
	// second key that does not do what it says.
	if f.picker.filter != "" && (f.at == screenBinding || f.at == screenExecEnv) {
		bindings = append(append([]keyBinding(nil), bindings[:len(bindings)-1]...),
			keyBinding{Key: "esc", Does: "clear filter"})
	}

	// Wrapped like everything else: a legend that runs off the edge hides the
	// key the user is looking for.
	return lipgloss.NewStyle().MarginLeft(2).Width(contentWidth(f.width)).Render(legend(bindings...))
}

// openEditor hands the manifest to the user's editor and reads back whatever
// they saved. What comes back is validated like any other file: an editor is
// a way to fix the answers, not a way around the ledger.
func (f flow) openEditor() tea.Cmd {
	file, err := os.CreateTemp("", "datarobot-*.yaml")
	if err != nil {
		return func() tea.Msg { return editedMsg{err: fmt.Errorf("cannot open an editor: %w", err)} }
	}

	name := file.Name()

	if _, err := file.Write(f.content); err != nil {
		_ = file.Close()
		_ = os.Remove(name)

		return func() tea.Msg { return editedMsg{err: fmt.Errorf("cannot open an editor: %w", err)} }
	}

	_ = file.Close()

	editor, args := editorCommand()
	dir := f.detected.Dir

	return tea.ExecProcess(exec.Command(editor, append(args, name)...), func(runErr error) tea.Msg {
		defer func() { _ = os.Remove(name) }()

		if runErr != nil {
			return editedMsg{err: fmt.Errorf("%s exited with an error: %w", editor, runErr)}
		}

		edited, err := os.ReadFile(name)
		if err != nil {
			return editedMsg{err: fmt.Errorf("cannot read back the edited manifest: %w", err)}
		}

		if err := checkRendered(edited, dir, authorUser); err != nil {
			return editedMsg{err: err}
		}

		return editedMsg{content: edited}
	})
}

// editorCommand names the editor to open, split the way a shell would so
// "code --wait" works as well as "vim".
//
// The choice comes from the CLI's own external-editor setting, which is where
// VISUAL and EDITOR are already mapped and where a configured preference or
// the flag can override them. Reading the two variables here directly would
// have made this the one editor in the CLI that a configured preference could
// not reach. The fallback below stays for callers that arrive without the root
// command's initialization, which is what leaves the setting unset.
func editorCommand() (string, []string) {
	editor := strings.TrimSpace(viperx.GetString("external-editor"))
	if editor == "" {
		return defaultEditor, nil
	}

	parts, err := splitCommand(editor)
	if err != nil || len(parts) == 0 {
		return defaultEditor, nil
	}

	return parts[0], parts[1:]
}

// unifiedDiff renders what changes about the running workload. An empty
// string means nothing does.
func unifiedDiff(name, before, after string) string {
	if before == after {
		return ""
	}

	return udiff.Unified(name+" (running)", name+" (this file)", before, after)
}

// pickedWorkload and pickedEnv are what a picker hands back when a row is
// chosen, carried on the row itself so the caller does not have to look it up
// again by name.
type pickedWorkload struct {
	id   string
	name string
}

type pickedEnv struct {
	id        string
	versionID string
}

// createNewLabel is the pinned row. It says what choosing it does, because
// "create a new workload" on its own reads as though something is created
// here, and nothing is: the name comes next and the workload comes at deploy.
const createNewLabel = "＋ Set up a new workload"

func newWorkloadPicker(workloads []workload.Workload, width, height int) rowTable {
	rows := make([]tableRow, 0, len(workloads)+1)
	rows = append(rows, tableRow{
		cells: []string{createNewLabel, "", ""},
		value: pickedWorkload{id: createNewID},
	})

	for _, w := range workloads {
		rows = append(rows, tableRow{
			cells: []string{w.Name, strings.ToLower(w.Status), w.UpdatedAt.Format("2 Jan 2006")},
			value: pickedWorkload{id: w.ID, name: w.Name},
		})
	}

	return newRowTable([]string{"WORKLOAD", "STATUS", "UPDATED"}, rows, true, width, height)
}

func newExecEnvPicker(environments []workload.ExecutionEnvironment, width, height int) rowTable {
	rows := make([]tableRow, 0, len(environments))

	for _, ee := range environments {
		// An environment with nothing built is not offered: picking it would
		// fail at build time with nothing the user could do. The listing
		// already filters these, but that invariant lives in another package
		// and this is the call site that would panic if it changed.
		if ee.LatestSuccessfulVersion == nil {
			continue
		}

		rows = append(rows, tableRow{
			cells: []string{ee.Name, languageLabel(ee.ProgrammingLanguage), ee.ID},
			value: pickedEnv{id: ee.ID, versionID: ee.LatestSuccessfulVersion.ID},
		})
	}

	return newRowTable([]string{"BASE IMAGE", "LANGUAGE", "ID"}, rows, true, width, height)
}

// languageLabel renders the language column: the platform's value as it came
// (only the sort folds case; a server that says "R" is shown saying "R"), and
// a dash where it says "other" or nothing — a word that means "unlabeled"
// reads better as absence than as a category.
func languageLabel(language string) string {
	trimmed := strings.TrimSpace(language)
	if lower := strings.ToLower(trimmed); lower == "" || lower == "other" {
		return "—"
	}

	return trimmed
}

// replicaValue leaves the field empty rather than showing a zero, which is
// how an autoscaled workload arrives and is not a count anyone typed.
func replicaValue(replicas int) string {
	if replicas < 1 {
		return ""
	}

	return strconv.Itoa(replicas)
}

// replicaPlaceholder says why the field is empty when the workload's own
// policy owns the number.
func replicaPlaceholder(autoscaled bool) string {
	if autoscaled {
		return "set by autoscaling"
	}

	return "1"
}

func newInput(value, placeholder string) textinput.Model {
	input := textinput.New()
	input.SetValue(value)
	input.Placeholder = placeholder
	input.PromptStyle = lipgloss.NewStyle()
	input.Focus()

	return input
}

func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}

	return strings.Join(lines, "\n") + "\n"
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}

	return "no"
}
