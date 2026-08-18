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

// namePlaceholder shows the shape of a workload name without proposing one.
// The directory is a poor suggestion often enough (src, app, cli, tmp) that
// offering it invites a name nobody chose, and this name ends up on a
// deployed workload.
const namePlaceholder = "my-app-name"

// runInteractiveFlow asks the questions and returns the manifest the user
// agreed to. The workload list is fetched up front because the first screen
// depends on it: with nothing to bind to, there is no binding question.
func runInteractiveFlow(opts Options, detected Detected) ([]byte, manifest.Draft, error) {
	// No screen can settle this one, so it is not a question the wizard gets
	// to ask. Started anyway, the error would sit behind the loading view
	// while the fetch ran, and arriving at the first screen would clear it
	// and drop --name without saying so.
	if err := opts.Answers.checkBinding(); err != nil {
		return nil, manifest.Draft{}, err
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
		inputs[fieldHealthPath] = newInput(f.draft.HealthPath, manifest.DefaultHealthPath)
		inputs[fieldReplicas] = newInput(replicaValue(f.draft.Runtime.Replicas), replicaPlaceholder(f.autoscaled()))
		inputs[fieldCPU] = newInput(formatCPU(f.draft.Runtime.CPU), "0.5")
		inputs[fieldMemory] = newInput(f.draft.Runtime.Memory, manifest.DefaultMemory)
		inputs[fieldImportance] = newInput(f.draft.Importance, manifest.DefaultImportance)

		f.inputs = inputs

		for i := range f.inputs[1:] {
			f.inputs[i+1].Blur()
		}

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

	return "These are " + liveStyle.Render(f.live.Name) +
		"'s current values. Changing one changes the workload."
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
	screenSettings:   "How should this workload run?",
	screenEnv:        "Which variables should the manifest declare?",
	screenConfirm:    "Ready to write " + manifest.FileName,
}

func (f flow) question() string {
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
	fieldHealthPath: "Readiness probe path. Traffic is withheld until this endpoint reports success.",
	fieldReplicas:   "Container instances to run. Reported as protons in status output and logs.",
	fieldCPU:        "CPU cores per replica. Fractional values are allowed.",
	fieldMemory:     "Memory per replica. A byte count or a 1000-based unit (" + strings.Join(manifest.MemoryUnits(), ", ") + "). Binary units such as Gi are rejected: they would be read as their decimal equivalents.",
	fieldImportance: "Scheduling priority when capacity is constrained: " + strings.Join(manifest.ImportanceLevels, ", ") + ".",
}

func (f flow) settingsView() string {
	var b strings.Builder

	width := 0
	for _, label := range settingsLabels {
		width = max(width, len(label))
	}

	for i, label := range settingsLabels {
		padded := label + strings.Repeat(" ", width-len(label))
		fmt.Fprintf(&b, "  %s  %s\n", labelStyle.Render(padded), f.inputs[i].View())
	}

	return strings.TrimRight(b.String(), "\n")
}

// settingsNote explains the field the cursor is on. The port's note carries
// its provenance too, since that is the one value detection can vouch for.
func (f flow) settingsNote() string {
	if f.focus < 0 || f.focus >= len(settingsNotes) {
		return ""
	}

	note := settingsNotes[f.focus]
	if f.focus == 0 {
		note += " " + capitalize(f.detected.PortSource) + "."
	}

	return note
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
	f.preview = viewport.New(contentWidth(f.width), min(previewHeight(f.height), lines))
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
	previewChrome        = 12
)

func previewHeight(terminalHeight int) int {
	if terminalHeight <= 0 {
		return defaultPreviewHeight
	}

	return max(terminalHeight-previewChrome, minPreviewHeight)
}

// confirmNote is what the user is agreeing to, beyond the file itself: where
// it goes, how much it will run with, and what happens next.
func (f flow) confirmNote() string {
	if f.failed != nil {
		return ""
	}

	return strings.Join([]string{
		"Writing to " + ShortPath(manifest.Path(f.detected.Dir)),
		f.runtimeLine(),
		f.nextStep(),
	}, "\n")
}

// runtimeLine summarises the sizing the file will carry. A bound workload
// reports its own, not the documented defaults, because this is the line the
// user is consenting to.
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

	return "Runtime: " + replicas + " · " + sizing + "    (edit the file for GPU/autoscaling)"
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
			cells: []string{ee.Name, ee.ID},
			value: pickedEnv{id: ee.ID, versionID: ee.LatestSuccessfulVersion.ID},
		})
	}

	return newRowTable([]string{"BASE IMAGE", "ID"}, rows, true, width, height)
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
