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
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// screen is one question. The order here is the order of the design's
// screens; which ones a given run visits depends on the answers.
type screen int

const (
	screenBinding screen = iota
	screenName
	screenKind
	screenA2A
	screenSource
	screenExecEnv
	screenEntrypoint
	screenImage
	screenSettings
	screenEnv
	screenConfirm
)

// flow is the wizard: one model for every screen, because the screens share
// one answer set and back-navigation between them has to be free.
type flow struct {
	detected  Detected
	workloads []workload.Workload
	// pendingBind is a workload id a flag named, fetched by Init before the
	// first screen so a flag-driven bind downloads the live spec exactly as
	// the picker would.
	pendingBind string
	// execEnvs is the base-image list once it has been fetched. It is kept so
	// walking back to that screen shows the list again instead of an empty
	// table: back-navigation re-enters a screen but does not re-run the work
	// that filled it.
	execEnvs []workload.ExecutionEnvironment

	// draft accumulates the answers. live is the workload being bound to,
	// nil when a new one is being created.
	draft manifest.Draft
	live  *manifest.Live

	// answers is what the flags said, kept because some of it is needed after
	// the run has started: binding downloads the live spec mid-flow, and the
	// answers have to be layered over it rather than replaced by it.
	answers Answers

	// at is the current screen; history is what Back walks, so a skipped
	// screen is skipped in both directions without a second rule set.
	at      screen
	history []screen

	choice   choice
	envTable envTable
	picker   rowTable
	inputs   []textinput.Model
	// focus is the settings screen's cursor: a field index, or advancedStop
	// when the row that opens the rest of the screen is what is focused.
	focus int
	// advancedOpen is whether that row is open. It survives leaving the screen
	// and coming back, so back-navigation returns to the screen the user left
	// rather than folding their own fields away again.
	advancedOpen bool

	// loading is the message shown while a screen waits on the network.
	loading string
	// failed is the last error, shown in place until the user moves.
	failed error

	// content is the manifest the confirm screen is showing, and diff is what
	// it changes when a live workload is bound.
	content []byte
	diff    string
	// preview scrolls whichever of those two the confirm screen shows. A
	// manifest is routinely taller than the terminal once a bound workload
	// brings its sidecars and environment across, and agreeing to a file you
	// cannot read to the end is not consent.
	preview viewport.Model

	// fatal ends the run with a reason rather than as a cancellation, for the
	// failures no screen can recover from.
	fatal error

	// imports is what storing the secrets did, read once the flow has ended
	// and the terminal is back. imported stops a second attempt: the confirm
	// screen can be reached twice, and a credential already stored must not be
	// stored again under a second name.
	imports  Import
	imported bool

	// dryRun stops the confirm screen storing anything. A run that promises
	// to write no file must not leave a credential behind either, and a
	// credential outlives the run that made it.
	dryRun bool

	// handEdited records that the confirm screen's [e] was used, so the file
	// about to be written is the user's bytes rather than a render of the
	// draft. It decides how the credential ids get in: re-rendering would
	// throw the edit away.
	handEdited bool

	// nameGiven distinguishes a name the user chose from the one the
	// directory suggested, so returning to the screen shows the answer that
	// was given there and never re-offers a suggestion as a typed value.
	nameGiven bool

	width, height int
	cancelled     bool
	done          bool
}

// liveLoadedMsg carries the downloaded workload, or the reason it could not
// be downloaded.
type liveLoadedMsg struct {
	live manifest.Live
	err  error
}

// execEnvsLoadedMsg carries the base images the picker offers.
type execEnvsLoadedMsg struct {
	environments []workload.ExecutionEnvironment
	err          error
}

// editedMsg carries the manifest as the user's editor left it.
type editedMsg struct {
	content []byte
	err     error
}

// secretsStoredMsg carries the variables with their credential ids filled in,
// and the record of which of them made it.
type secretsStoredMsg struct {
	vars   []manifest.EnvVar
	report Import
}

func newFlow(detected Detected, workloads []workload.Workload, answers Answers) flow {
	f := flow{detected: detected, workloads: workloads, answers: answers, pendingBind: answers.WorkloadID}

	draft, err := answers.draft(detected)
	if err != nil {
		// Interactively an unanswerable flag set is not fatal: the screens
		// exist to answer exactly this. Everything the flags did settle is
		// kept.
		draft = answers.partialDraft(detected)
	}

	f.draft = draft
	// A name passed as a flag is an answer the user gave, so the screen shows
	// it as a value rather than re-offering the directory's suggestion.
	f.nameGiven = answers.Name != ""
	f.at = f.first(answers)
	f.enter(f.at)

	// Only a flag the wizard cannot ask about is worth reporting on arrival.
	// "No Dockerfile, so the image source cannot be guessed" is a headless
	// error: interactively it is simply the question Q4 is about to ask, and
	// showing it on the first screen makes the wizard open on a complaint
	// about something the user has not been asked yet.
	f.failed = answers.check()

	if f.pendingBind != "" {
		f.loading = "Reading workload " + f.pendingBind + "…"
	}

	return f
}

// defaultDraft is where a run starts when nothing else has been said: the
// project's own suggestions and the documented defaults.
func defaultDraft(detected Detected) manifest.Draft {
	return Answers{}.partialDraft(detected)
}

// first picks the opening screen. Binding is skipped when there is nothing to
// bind to, or when a flag already said which workload this is.
func (f flow) first(answers Answers) screen {
	// A named workload is fetched by Init, and the kind screen is where the
	// questions resume once it arrives.
	if answers.WorkloadID != "" {
		return screenKind
	}

	if len(f.workloads) == 0 || answers.Name != "" {
		return screenName
	}

	return screenBinding
}

func (f flow) Init() tea.Cmd {
	if f.pendingBind == "" {
		return nil
	}

	return fetchLiveCmd(f.pendingBind)
}

// resizeComponents refits whatever has been built to the terminal's real
// size. Each component fixes its columns and height when it is constructed,
// and the first size message arrives after the opening screen already exists,
// so recording the numbers alone would leave that screen at the fallback for
// its whole life and later resizes would never reach the current screen
// either. Everything built so far is refitted rather than only the visible
// one, so walking back to a screen does not find it at the old size.
func (f *flow) resizeComponents() {
	f.picker.resize(f.width, f.height)
	f.envTable.resize(f.width, f.height)

	if len(f.content) > 0 {
		f.buildPreview()
	}
}

// fetchLiveCmd downloads a workload so the file can say everything it says.
func fetchLiveCmd(workloadID string) tea.Cmd {
	return func() tea.Msg {
		live, err := fetchLive(workloadID)

		return liveLoadedMsg{live: live, err: err}
	}
}

func (f flow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.resizeComponents()

		return f, nil

	case liveLoadedMsg:
		return f.liveLoaded(msg)

	case execEnvsLoadedMsg:
		return f.execEnvsLoaded(msg)

	case editedMsg:
		return f.edited(msg)

	case secretsStoredMsg:
		return f.secretsStored(msg)

	case tea.KeyMsg:
		return f.key(msg)
	}

	return f.delegate(msg)
}

// key routes a keystroke: navigation first, then whatever the current screen
// makes of it.
func (f flow) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if f.loading != "" {
		return f, nil
	}

	if f.routeToPicker(msg) {
		return f, nil
	}

	f.failed = nil

	switch msg.String() {
	case "esc":
		return f.back()
	case "enter":
		// On the advanced row enter is the row's own action. The screen is
		// still submitted with enter from any of its fields, which is where the
		// cursor starts.
		if f.onAdvancedRow() {
			f.advancedOpen = !f.advancedOpen

			return f, nil
		}

		return f.advance()
	}

	if f.at == screenConfirm && msg.String() == "e" {
		return f, f.openEditor()
	}

	return f.delegate(msg)
}

// routeToPicker gives a picker screen first refusal on a key, because typing
// narrows the list. Enter and esc stay with the flow: enter selects the row,
// and esc clears a filter before it goes back.
func (f *flow) routeToPicker(msg tea.KeyMsg) bool {
	if f.at != screenBinding && f.at != screenExecEnv {
		return false
	}

	if msg.String() == "esc" {
		return f.picker.clearFilter()
	}

	if msg.Type == tea.KeyEnter {
		return false
	}

	return f.picker.update(msg)
}

// delegate hands the message to whichever component the current screen shows.
func (f flow) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch f.at {
	case screenBinding, screenExecEnv:
		if key, ok := msg.(tea.KeyMsg); ok {
			f.picker.update(key)
		}

		return f, nil

	case screenKind, screenA2A, screenSource, screenEnv:
		if key, ok := msg.(tea.KeyMsg); ok {
			f.updateSelection(key)
		}

		return f, nil

	case screenName, screenEntrypoint, screenImage, screenSettings:
		return f.delegateToInput(msg)

	case screenConfirm:
		var cmd tea.Cmd

		f.preview, cmd = f.preview.Update(msg)

		return f, cmd
	}

	return f, nil
}

// delegateToInput routes a message on the text-entry screens: to the focus
// ring when the key moves between fields, and to the focused field otherwise.
func (f flow) delegateToInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)

	if isKey && f.at == screenSettings && isFocusKey(key) {
		return f, f.moveFocus(key)
	}

	// The advanced row is not a field, so a keystroke on it has nowhere to be
	// typed. Without this it would go into whichever input the index happened
	// to reach, or panic on the one it does not have. Only keystrokes are
	// swallowed: everything else routed here belongs to a component that is
	// still on screen, and dropping it would end whatever it is driving.
	if isKey && f.onAdvancedRow() {
		return f, nil
	}

	if f.focus < 0 {
		return f, nil
	}

	var cmd tea.Cmd

	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)

	return f, cmd
}

// updateSelection routes a key to whichever selection component the current
// screen shows.
func (f *flow) updateSelection(key tea.KeyMsg) {
	if f.at == screenEnv {
		f.envTable.update(key)

		return
	}

	f.choice.update(key)
}

// back returns to the previous screen, or cancels when there is none: esc on
// the first screen is the same "write nothing" the confirm screen offers.
func (f flow) back() (tea.Model, tea.Cmd) {
	if len(f.history) == 0 {
		f.cancelled = true

		return f, tea.Quit
	}

	f.at = f.history[len(f.history)-1]
	f.history = f.history[:len(f.history)-1]
	f.enter(f.at)

	// Arriving is arriving, whichever direction it came from: a screen that
	// fills itself on arrival has to do so on the way back too.
	return f, f.onEnter(f.at)
}

// advance records the current screen's answer and moves on. A screen that
// cannot accept its answer keeps the user where they are, with the reason.
func (f flow) advance() (tea.Model, tea.Cmd) {
	cmd, err := f.accept()
	if err != nil {
		f.failed = err

		return f, nil
	}

	if cmd != nil {
		return f, cmd
	}

	if f.at == screenConfirm {
		if f.failed != nil || len(f.content) == 0 {
			// render() failed on the way in. Ending as a cancellation would
			// replace the reason on screen with "nothing was written", so the
			// reason is carried out instead.
			f.fatal = f.renderFailure()

			return f, tea.Quit
		}

		f.done = true

		return f, tea.Quit
	}

	next := f.next()
	f.history = append(f.history, f.at)
	f.at = next
	f.enter(next)

	// onEnter mutates f (it can set the loading state), so it runs before the
	// model is returned rather than inside the return expression.
	cmd = f.onEnter(next)

	return f, cmd
}

// nextScreen is the flow when the answer does not change it.
var nextScreen = map[screen]screen{
	screenBinding:    screenName,
	screenName:       screenKind,
	screenKind:       screenSource,
	screenA2A:        screenSource,
	screenSource:     screenSettings,
	screenExecEnv:    screenEntrypoint,
	screenEntrypoint: screenSettings,
	screenImage:      screenSettings,
	screenSettings:   screenConfirm,
	screenEnv:        screenConfirm,
	screenConfirm:    screenConfirm,
}

// renderFailure is why the confirm screen has nothing to write.
func (f flow) renderFailure() error {
	if f.failed != nil {
		return f.failed
	}

	return errors.New("nothing to write: the manifest could not be rendered")
}

// next is the flow: the table above, plus the four answers that change it.
func (f flow) next() screen {
	if next, ok := f.branch(); ok {
		return next
	}

	return nextScreen[f.at]
}

// branch is the four answers that change where the flow goes next.
func (f flow) branch() (screen, bool) {
	switch f.at {
	case screenBinding:
		// A bound workload is already named.
		return screenKind, f.live != nil
	case screenKind:
		return screenA2A, f.draft.Type == manifest.TypeAgent
	case screenSource:
		return f.afterSource()
	case screenSettings:
		// --skip-env is an answer, not a headless-only shortcut: showing the
		// screen anyway would ask a question the user already declined, and
		// accepting it refills the EnvVars the flag deliberately emptied.
		return screenEnv, f.detected.HasEnvFile() && !f.answers.SkipEnv
	case screenName, screenA2A, screenExecEnv, screenEntrypoint, screenImage, screenEnv, screenConfirm:
		// These always go where the table says.
		return 0, false
	}

	return 0, false
}

func (f flow) afterSource() (screen, bool) {
	switch f.draft.Build.Mode {
	case manifest.BuildModeGenerated:
		return screenExecEnv, true
	case manifest.BuildModeImage:
		return screenImage, true
	default:
		return 0, false
	}
}

// accepting maps each screen to what recording its answer means. Screens
// absent from the table (the confirm screen) have nothing to record.
var accepting = map[screen]func(*flow) (tea.Cmd, error){
	screenBinding:    (*flow).acceptBinding,
	screenName:       (*flow).acceptName,
	screenKind:       (*flow).acceptKind,
	screenA2A:        (*flow).acceptA2A,
	screenSource:     (*flow).acceptSource,
	screenExecEnv:    (*flow).acceptExecEnv,
	screenEntrypoint: (*flow).acceptEntrypoint,
	screenImage:      (*flow).acceptImage,
	screenSettings:   (*flow).acceptSettings,
	screenEnv:        (*flow).acceptEnv,
	screenConfirm:    (*flow).acceptConfirm,
}

// acceptConfirm stores the secrets, and is the last thing that happens before
// the file is written.
//
// Here rather than when the env table is left, because a credential outlives
// the run that created it: someone who walks the wizard to the end and then
// cancels should not have left a secret on the platform. The cost is that the
// confirm screen shows the reference carrying the placeholder while the file
// written a moment later carries the id. That is the right way round; the
// screen is where a secret's value could leak, and no value is on it either
// way.
func (f *flow) acceptConfirm() (tea.Cmd, error) {
	// A dry run stores nothing, the same as the headless path. The references
	// keep their placeholders, which is what a file nobody created a
	// credential for should say.
	if f.imported || f.dryRun || pendingSecrets(f.draft.EnvVars) == 0 {
		return nil, nil
	}

	f.loading = "Storing secrets…"

	vars, detected, name := f.draft.EnvVars, f.detected, f.draft.Name

	return func() tea.Msg {
		stored, report := importSecrets(vars, detected, name)

		return secretsStoredMsg{vars: stored, report: report}
	}, nil
}

// secretsStored records the ids and puts them in the file, so what is written
// is what was on screen with the placeholders resolved. A failure now is
// fatal: the credentials exist, and ending as a cancellation would say nothing
// happened.
func (f flow) secretsStored(msg secretsStoredMsg) (tea.Model, tea.Cmd) {
	f.loading = ""
	f.imported = true
	f.imports = msg.report
	f.draft.EnvVars = msg.vars

	if err := f.resolveContent(); err != nil {
		f.fatal = err

		return f, tea.Quit
	}

	f.done = true

	return f, tea.Quit
}

// resolveContent puts the new credential ids into what is about to be written.
//
// A file nobody touched is re-rendered from the draft, which now carries them.
// One edited by hand on the confirm screen is not: re-rendering would throw
// that edit away silently, at the last moment before the write, including any
// credential id the user pasted in themselves. The placeholders in their own
// bytes are rewritten instead.
func (f *flow) resolveContent() error {
	if !f.handEdited {
		return f.render()
	}

	resolved, err := manifest.ResolveCredentials(f.content, credentialIDs(f.draft.EnvVars))
	if err != nil {
		return err
	}

	f.content = resolved

	return f.rediff()
}

// credentialIDs is the id stored for each variable, by variable name.
func credentialIDs(vars []manifest.EnvVar) map[string]string {
	ids := make(map[string]string, len(vars))

	for _, v := range vars {
		if v.Secret && v.CredentialID != "" {
			ids[v.Name] = v.CredentialID
		}
	}

	return ids
}

// accept validates and records the current screen's answer. A non-nil command
// means the answer needs the network before the flow can move.
func (f *flow) accept() (tea.Cmd, error) {
	accept, ok := accepting[f.at]
	if !ok {
		return nil, nil
	}

	return accept(f)
}

// acceptKind records the kind. Only a changed answer resets the flags that
// belong to the old kind, so re-confirming "agent" on the way back does not
// silently withdraw the A2A answer given on the next screen.
func (f *flow) acceptKind() (tea.Cmd, error) {
	if kind := f.choice.value(); kind != f.draft.Type {
		f.draft.Type = kind
		f.draft.A2AEnabled = false
	}

	return nil, nil
}

func (f *flow) acceptA2A() (tea.Cmd, error) {
	f.draft.A2AEnabled = f.choice.value() == "yes"

	return nil, nil
}

// acceptEnv records the table's per-row answers. Names only, commented out,
// so nothing about the deploy changes either way; what the user is settling
// is which keys the file mentions and which of them the credential import
// will later be pointed at.
func (f *flow) acceptEnv() (tea.Cmd, error) {
	f.draft.EnvVars = f.envTable.vars()

	return nil, nil
}

// acceptSource records the image source. Only a changed answer clears the
// fields that belong to the old one: re-confirming the source a bound
// workload already uses must keep its image and its base-image ids.
func (f *flow) acceptSource() (tea.Cmd, error) {
	mode := f.choice.value()

	// Refusing before recording keeps a rejected pick from destroying the
	// answer the screen is about to be shown again with.
	if mode == manifest.BuildModeDockerfile && !f.detected.HasDockerfile {
		return nil, fmt.Errorf("no %s in %s: pick another source or add one", DockerfileName, f.detected.Dir)
	}

	if mode != f.draft.Build.Mode {
		f.draft.Build = manifest.Build{Mode: mode}
	}

	return nil, nil
}

func (f *flow) acceptBinding() (tea.Cmd, error) {
	row := f.picker.selected()
	if row == nil {
		return nil, errors.New("nothing selected")
	}

	item, ok := row.value.(pickedWorkload)
	if !ok {
		return nil, errors.New("nothing selected")
	}

	if item.id == createNewID {
		// Start over rather than carrying the previous pick's name, kind and
		// image forward: "create a new workload" after looking at an existing
		// one must not prefill a copy of it. Starting over means from this
		// run's own flags, not from nothing: a flag the user passed is an
		// answer they gave, and looking at a workload and declining it is not
		// a reason to withdraw it.
		if f.live != nil {
			f.live = nil
			f.draft = f.answers.partialDraft(f.detected)
		}

		f.draft.WorkloadID = ""

		return nil, nil
	}

	// Binding is the one answer that costs a round trip: the file has to say
	// everything the running workload says, and only the server knows that.
	f.loading = "Reading " + item.name + "…"

	return fetchLiveCmd(item.id), nil
}

// acceptName records the name, which has to be typed. There is no default:
// the directory is a poor proposal often enough that accepting it by reflex
// would put a name like "src" on a deployed workload, and unlike every other
// screen this answer is not something detection can know.
//
// A headless run does default to the directory, because there is nobody to
// ask and the flags have already said not to prompt.
func (f *flow) acceptName() (tea.Cmd, error) {
	typed := strings.TrimSpace(f.inputs[0].Value())
	if typed == "" {
		return nil, errors.New("a name is required")
	}

	f.draft.Name = typed
	f.nameGiven = true

	return nil, nil
}

func (f *flow) acceptExecEnv() (tea.Cmd, error) {
	row := f.picker.selected()
	if row == nil {
		return nil, errors.New("nothing selected")
	}

	item, ok := row.value.(pickedEnv)
	if !ok {
		return nil, errors.New("nothing selected")
	}

	f.draft.Build.ExecutionEnvironmentID = item.id
	f.draft.Build.ExecutionEnvironmentVersionID = item.versionID

	return nil, nil
}

func (f *flow) acceptEntrypoint() (tea.Cmd, error) {
	entrypoint, err := splitCommand(f.inputs[0].Value())
	if err != nil {
		return nil, fmt.Errorf("entrypoint %w", err)
	}

	f.draft.Build.Entrypoint = entrypoint

	return nil, nil
}

func (f *flow) acceptImage() (tea.Cmd, error) {
	image := strings.TrimSpace(f.inputs[0].Value())
	if image == "" {
		return nil, errors.New("an image URI is required")
	}

	f.draft.Build.ImageURI = image

	return nil, nil
}

// acceptReplicas reads the replica count, which is not a question at all for
// a workload that scales itself: its policy decides the number, applyRuntime
// drops any answer given while that policy is active, and the field arrives
// seeded with the zero that says so. Demanding "1 or more" there would trap
// the screen behind a value that could not be used, and taking a number would
// promise something the file will not carry.
func (f flow) acceptReplicas() (int, error) {
	text := strings.TrimSpace(f.field(fieldReplicas))

	if f.autoscaled() {
		if text == "" || text == "0" {
			return 0, nil
		}

		return 0, errors.New("this workload scales itself, so its replica count comes from that policy; leave the field at 0")
	}

	replicas, err := strconv.Atoi(text)
	if err != nil || replicas < 1 {
		return 0, errors.New("replicas must be a whole number, 1 or more")
	}

	return replicas, nil
}

// acceptCPU reads the cores field. ParseFloat takes NaN and Inf, and neither
// of them is <= 0, so a plain positivity check lets both through to a file
// carrying a quantity nothing can schedule.
func (f flow) acceptCPU() (float64, error) {
	cpu, err := strconv.ParseFloat(strings.TrimSpace(f.field(fieldCPU)), 64)
	if err != nil || cpu <= 0 || math.IsNaN(cpu) || math.IsInf(cpu, 0) {
		return 0, errors.New("cpu must be a positive number, such as 0.5 or 2")
	}

	return cpu, nil
}

// autoscaled reports whether the bound workload manages its own replica count.
func (f flow) autoscaled() bool {
	return f.live != nil && f.live.Autoscaled()
}

// acceptSettings validates every field before recording any of them, so a
// screen that is refused leaves the answers exactly as the user left them.
// Fields behind the advanced row are validated too, since a bad --cpu has to
// fail here rather than at write time, and a refusal there opens the row: an
// error naming a field nobody can see is a screen with no way forward.
func (f *flow) acceptSettings() (tea.Cmd, error) {
	port, err := strconv.Atoi(f.field(fieldPort))
	if err != nil {
		return nil, errors.New("the port must be a number")
	}

	if err := checkPort(port, "port"); err != nil {
		return nil, err
	}

	// An empty path is the answer "no readiness probe", which the file records
	// by carrying no probe block at all. A path that is set still has to be one.
	path := f.field(fieldHealthPath)
	if path != "" && !strings.HasPrefix(path, "/") {
		return nil, f.reveal(fieldHealthPath,
			errors.New("the health path must start with /, or leave it empty to run without a probe"))
	}

	replicas, err := f.acceptReplicas()
	if err != nil {
		return nil, f.reveal(fieldReplicas, err)
	}

	cpu, err := f.acceptCPU()
	if err != nil {
		return nil, f.reveal(fieldCPU, err)
	}

	memory := f.field(fieldMemory)
	if !manifest.ValidMemory(memory) {
		return nil, f.reveal(fieldMemory, fmt.Errorf(
			"memory must be a byte count or a 1000-based unit (%s), such as 512MB or 4GB",
			strings.Join(manifest.MemoryUnits(), ", ")))
	}

	memory = manifest.NormalizeMemory(memory)

	// An empty field takes the documented default, the same as the headless
	// path, rather than being the one field on this screen where clearing is
	// fatal.
	importance := orDefault(strings.ToLower(f.field(fieldImportance)), manifest.DefaultImportance)
	if !manifest.ValidImportance(importance) {
		return nil, f.reveal(fieldImportance, fmt.Errorf("importance must be one of %s",
			strings.Join(manifest.ImportanceLevels, ", ")))
	}

	f.draft.Port = port
	f.draft.HealthPath = path
	f.draft.Importance = importance
	f.draft.Runtime = manifest.Runtime{Replicas: replicas, CPU: cpu, Memory: memory}

	return nil, nil
}

// reveal puts the cursor on the field a validation error is about, opening the
// advanced row when that is where the field lives. The error is returned
// unchanged so the caller reads as one statement.
func (f *flow) reveal(field int, err error) error {
	if field >= firstAdvancedField {
		f.advancedOpen = true
	}

	f.focus = field
	f.applyFocus()

	return err
}

// field is the trimmed contents of one input on the current screen.
func (f flow) field(i int) string {
	return strings.TrimSpace(f.inputs[i].Value())
}

// onEnter starts whatever the screen needs before it can be answered.
func (f *flow) onEnter(at screen) tea.Cmd {
	switch at {
	case screenExecEnv:
		if len(f.execEnvs) > 0 {
			// Already fetched, and enter() has rebuilt the table from it.
			return nil
		}

		f.loading = "Loading base images…"

		return func() tea.Msg {
			environments, err := listExecEnvsFn(execEnvPickLimit)

			return execEnvsLoadedMsg{environments: environments, err: err}
		}

	case screenConfirm:
		if err := f.render(); err != nil {
			f.failed = err
		}

		return nil

	case screenBinding, screenName, screenKind, screenA2A, screenSource,
		screenEntrypoint, screenImage, screenSettings, screenEnv:
		// Everything these screens show is already in hand.
		return nil
	}

	return nil
}

// render produces the file the confirm screen shows, and for a bound workload
// the diff against what is running: confirming is consenting to change
// something live, so the change is what gets shown.
func (f *flow) render() error {
	// Cleared first: a screen that rendered once and then fails on a later
	// visit must not offer the earlier draft's file as though it were the
	// current answers.
	f.content, f.diff = nil, ""

	if f.live == nil {
		content, err := f.draft.Render()
		if err != nil {
			return err
		}

		f.content = content
		f.diff = ""
		f.buildPreview()

		return nil
	}

	before, err := f.live.Render()
	if err != nil {
		return err
	}

	// A name the workload already declares keeps its running value, so it is
	// not one this run adds. Narrowing before Apply rather than leaving it to
	// Apply's own skip is what keeps the summary the command prints equal to
	// what reached the file.
	f.draft.EnvVars = f.live.NewEnvVars(f.draft.EnvVars)

	applied, err := f.live.Apply(f.draft)
	if err != nil {
		return err
	}

	content, err := applied.Render()
	if err != nil {
		return err
	}

	f.content = content
	f.diff = unifiedDiff(f.live.Name, string(before), string(content))
	f.buildPreview()

	return nil
}

func (f flow) liveLoaded(msg liveLoadedMsg) (tea.Model, tea.Cmd) {
	f.loading = ""

	if msg.err != nil {
		f.failed = msg.err

		// A flag-named workload that cannot be read leaves nothing to ask
		// about: there is no binding screen to fall back to, and continuing
		// would write a from-scratch file under a live workload's id.
		if f.pendingBind != "" {
			f.fatal = msg.err

			return f, tea.Quit
		}

		return f, nil
	}

	live := msg.live
	f.live = &live
	// The live spec is the new set of defaults, not the new set of answers:
	// --replicas and --memory were given before the fetch and still stand.
	// Replacing the draft outright would drop every flag the wizard has no
	// screen to re-ask, which is the same layering the headless bind does.
	f.draft = f.answers.partialApplyTo(live.Defaults(), f.detected)
	f.pendingBind = ""

	// Only the binding screen has somewhere to go back to; a flag-driven bind
	// starts here.
	if f.at == screenBinding {
		f.history = append(f.history, screenBinding)
	}

	f.at = screenKind
	f.enter(f.at)

	return f, nil
}

func (f flow) execEnvsLoaded(msg execEnvsLoadedMsg) (tea.Model, tea.Cmd) {
	f.loading = ""

	switch {
	case msg.err != nil:
		f.failed = fmt.Errorf("cannot list base images: %w", msg.err)
	case len(msg.environments) == 0:
		f.failed = errors.New("no execution environments are available; go back and pick another image source")
	default:
		f.execEnvs = msg.environments
		f.picker = newExecEnvPicker(msg.environments, f.width, f.height)
	}

	return f, nil
}

func (f flow) edited(msg editedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		f.failed = msg.err

		return f, nil
	}

	f.content = msg.content
	f.handEdited = true

	// The diff has to be recomputed against what the editor left, not
	// cleared: an empty diff is how the screen says "nothing changes for the
	// running workload", which an edited file has no business claiming.
	if err := f.rediff(); err != nil {
		f.failed = err

		return f, nil
	}

	return f, nil
}

// rediff recomputes what the confirm screen shows against the current bytes,
// which is only meaningful when a live workload is being changed.
func (f *flow) rediff() error {
	if f.live != nil {
		before, err := f.live.Render()
		if err != nil {
			return err
		}

		f.diff = unifiedDiff(f.live.Name, string(before), string(f.content))
	}

	f.buildPreview()

	return nil
}

func (f flow) result() ([]byte, manifest.Draft, error) {
	switch {
	case f.fatal != nil:
		return nil, manifest.Draft{}, f.fatal
	case f.cancelled, !f.done:
		return nil, manifest.Draft{}, ErrCancelled
	case len(f.content) == 0:
		// Reachable only if a render failure slipped past the confirm screen;
		// reporting it beats handing back an empty file to be written.
		return nil, manifest.Draft{}, errors.New("nothing to write: the manifest could not be rendered")
	}

	return f.content, f.draft, nil
}

func isFocusKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab", "shift+tab", "up", "down":
		return true
	}

	return false
}

// moveFocus walks the settings screen's stops: the port, the advanced row, and
// the fields that row reveals while it is open. Only the settings screen has
// more than one thing to move between, which is why this is not reached from
// the other input screens.
func (f *flow) moveFocus(msg tea.KeyMsg) tea.Cmd {
	stops := f.settingsStops()

	// Not found and the first stop are different things, and slices.Index
	// spells both -1 and the sentinel this ring uses the same way. A focus that
	// is not a stop is a broken invariant rather than a reason to move, so it
	// restarts the walk deliberately instead of being clamped into one.
	at := slices.Index(stops, f.focus)
	if at < 0 {
		at = 0
	}

	step := 1
	if msg.String() == "shift+tab" || msg.String() == "up" {
		step = -1
	}

	f.focus = stops[(at+step+len(stops))%len(stops)]

	return f.applyFocus()
}

// settingsStops is the order the cursor visits, built from the one field order
// so a field added to it needs no second edit here.
func (f flow) settingsStops() []int {
	stops := []int{fieldPort, advancedStop}

	if !f.advancedOpen {
		return stops
	}

	for field := firstAdvancedField; field < settingsFieldCount; field++ {
		stops = append(stops, field)
	}

	return stops
}

// applyFocus puts the cursor in the focused field, and in none of them when
// the advanced row is what is focused.
//
// The command it returns starts the cursor blinking in the newly focused
// field, and has to reach the caller: dropped, the field keeps a static block
// where the caret should be, which reads as a field that cannot be typed into.
func (f *flow) applyFocus() tea.Cmd {
	var cmd tea.Cmd

	for i := range f.inputs {
		if i == f.focus {
			cmd = f.inputs[i].Focus()

			continue
		}

		f.inputs[i].Blur()
	}

	return cmd
}
