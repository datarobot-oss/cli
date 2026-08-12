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
	focus    int

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
		if key, ok := msg.(tea.KeyMsg); ok && f.at == screenSettings && isFocusKey(key) {
			f.moveFocus(key)

			return f, nil
		}

		var cmd tea.Cmd

		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)

		return f, cmd

	case screenConfirm:
		var cmd tea.Cmd

		f.preview, cmd = f.preview.Update(msg)

		return f, cmd
	}

	return f, nil
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
		// one must not prefill a copy of it.
		if f.live != nil {
			f.live = nil
			f.draft = defaultDraft(f.detected)
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
	text := strings.TrimSpace(f.field(2))

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
	cpu, err := strconv.ParseFloat(strings.TrimSpace(f.field(3)), 64)
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
func (f *flow) acceptSettings() (tea.Cmd, error) {
	port, err := strconv.Atoi(f.field(0))
	if err != nil {
		return nil, errors.New("the port must be a number")
	}

	if err := checkPort(port, "port"); err != nil {
		return nil, err
	}

	path := f.field(1)
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("the health path must start with /")
	}

	replicas, err := f.acceptReplicas()
	if err != nil {
		return nil, err
	}

	cpu, err := f.acceptCPU()
	if err != nil {
		return nil, err
	}

	memory := f.field(4)
	if !manifest.ValidMemory(memory) {
		return nil, fmt.Errorf("memory must be a byte count or a 1000-based unit (%s), such as 512MB or 4GB",
			strings.Join(manifest.MemoryUnits(), ", "))
	}

	memory = manifest.NormalizeMemory(memory)

	importance := strings.ToLower(f.field(5))
	if !manifest.ValidImportance(importance) {
		return nil, fmt.Errorf("importance must be one of %s",
			strings.Join(manifest.ImportanceLevels, ", "))
	}

	f.draft.Port = port
	f.draft.HealthPath = path
	f.draft.Importance = importance
	f.draft.Runtime = manifest.Runtime{Replicas: replicas, CPU: cpu, Memory: memory}

	return nil, nil
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

	// The diff has to be recomputed against what the editor left, not
	// cleared: an empty diff is how the screen says "nothing changes for the
	// running workload", which an edited file has no business claiming.
	if f.live != nil {
		before, err := f.live.Render()
		if err != nil {
			f.failed = err

			return f, nil
		}

		f.diff = unifiedDiff(f.live.Name, string(before), string(f.content))
	}

	f.buildPreview()

	return f, nil
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

func (f *flow) moveFocus(msg tea.KeyMsg) {
	f.inputs[f.focus].Blur()

	if msg.String() == "shift+tab" || msg.String() == "up" {
		f.focus = (f.focus - 1 + len(f.inputs)) % len(f.inputs)
	} else {
		f.focus = (f.focus + 1) % len(f.inputs)
	}

	f.inputs[f.focus].Focus()
}
