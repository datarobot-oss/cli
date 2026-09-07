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
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// What a row of the preview says is going to happen to a variable.
const (
	actAdd    = "add"
	actUpdate = "update"
	actResend = "re-send"
	actSkip   = "skip"
)

// envAction is one variable and what the sync would do about it.
type envAction struct {
	name   string
	act    string
	detail string
}

// envPlan is every variable the two files have anything to say about, and what
// a sync would do with each, worked out before anything is written or sent.
//
// It exists because one flag now does what two used to. Splitting the acts
// made the reader work out which half applied to what; collapsing them without
// showing the result would mean a single word on the command line silently
// rewriting a committed file and overwriting values on the tenant. So the run
// says what it is about to do, and asks.
//
// Skipped rows are the point as much as the others. A variable this will not
// touch, and why, is the question people actually have about a reconciliation:
// most of all what happens to a name the manifest carries and .env no longer
// mentions, which is nothing, and which is invisible unless it is written down.
//
// The re-send rows are candidates rather than certainties. Proving a
// credential belongs to this project takes a request per variable, and the
// apply already refuses one that does not and says so by name; spending those
// round trips twice to make a preview marginally more precise is not worth the
// wait in front of a prompt.
func (o Options) envPlan(parsed *manifest.Manifest, detected Detected, wanted []manifest.EnvVar) []envAction {
	declared := parsed.EnvVarNames()
	blocked := parsed.CanDeclareEnvVars() != nil

	actions := o.addActions(wanted, declared, blocked)
	actions = append(actions, o.valueActions(parsed, detected)...)

	return append(actions, o.skipActions(parsed, detected, declared)...)
}

// addActions is the names .env has that the manifest does not.
func (o Options) addActions(wanted []manifest.EnvVar, declared map[string]bool, blocked bool) []envAction {
	actions := make([]envAction, 0, len(wanted))

	for _, v := range undeclared(wanted, declared) {
		if blocked {
			actions = append(actions, envAction{
				v.Name, actSkip,
				"the manifest's environment is shared, so nothing can be added to it",
			})

			continue
		}

		detail := "written into " + manifest.FileName + " as a literal"
		if v.Secret {
			detail = "stored as a new credential, and referenced by id"
		}

		actions = append(actions, envAction{v.Name, actAdd, detail})
	}

	return actions
}

// valueActions is the declared names whose value this run would move: a
// literal rewritten in the file, and a secret sent to the store it came from.
func (o Options) valueActions(parsed *manifest.Manifest, detected Detected) []envAction {
	changed, unverifiable, _, _, _ := o.compareValues(parsed, detected)

	actions := make([]envAction, 0, len(changed)+len(unverifiable))

	for _, name := range changed {
		actions = append(actions, envAction{
			name, actUpdate,
			"its value in " + manifest.FileName + " is rewritten to match " + EnvFileName,
		})
	}

	for _, name := range unverifiable {
		actions = append(actions, envAction{
			name, actResend,
			"its value goes to the credential store; " + manifest.FileName + " is not changed",
		})
	}

	return actions
}

// skipActions is everything the sync will leave exactly as it found it, and
// why. A reconciliation is judged as much by what it does not touch.
func (o Options) skipActions(parsed *manifest.Manifest, detected Detected, declared map[string]bool) []envAction {
	_, _, reclassified, structured, heldBack := o.compareValues(parsed, detected)

	actions := make([]envAction, 0, len(reclassified)+len(structured)+len(heldBack))

	for _, name := range reclassified {
		actions = append(actions, envAction{
			name, actSkip,
			"a literal where the manifest holds a credential, or the reverse",
		})
	}

	for _, name := range structured {
		actions = append(actions, envAction{
			name, actSkip,
			"the manifest holds it as a mapping or a list, which no rewrite touches",
		})
	}

	for _, name := range heldBack {
		actions = append(actions, envAction{
			name, actSkip,
			EnvFileName + " now reads its value as local-only",
		})
	}

	for _, v := range detected.EnvVars {
		if v.Kind == EnvLocal && !declared[v.Name] {
			actions = append(actions, envAction{v.Name, actSkip, "read as local-only, so it is left out"})
		}
	}

	for _, name := range parsed.PendingEnvNames() {
		actions = append(actions, envAction{
			name, actSkip,
			"still names the credential placeholder, which a deploy refuses",
		})
	}

	return append(actions, droppedActions(parsed, detected)...)
}

// droppedActions is the names the manifest declares that .env no longer
// mentions. Nothing happens to them, and that is the answer to the question a
// reconciliation raises loudest: removing a line from your own copy of a file
// says nothing about what the workload should run, so the sync is
// one-directional and says so out loud rather than by omission.
func droppedActions(parsed *manifest.Manifest, detected Detected) []envAction {
	local := make(map[string]bool, len(detected.EnvVars))
	for _, v := range detected.EnvVars {
		local[v.Name] = true
	}

	var actions []envAction

	for _, declared := range parsed.DeclaredEnvVars() {
		if !local[declared.Name] {
			actions = append(actions, envAction{
				declared.Name, actSkip,
				"not in " + EnvFileName + "; nothing is ever removed",
			})
		}
	}

	return actions
}

// changes reports whether the plan would do anything at all, so a run with
// only skipped rows does not stop to ask about work it is not going to do.
func changes(actions []envAction) bool {
	for _, a := range actions {
		if a.act != actSkip {
			return true
		}
	}

	return false
}

// renderEnvPlan draws the plan as the table the user is asked to approve.
//
// One table rather than a section per act, because the reader's question is
// about a variable ("what happens to API_KEY") rather than about a category,
// and a name appears exactly once.
func renderEnvPlan(actions []envAction) string {
	rows := make([][]string, 0, len(actions))
	for _, a := range actions {
		rows = append(rows, []string{a.name, a.act, a.detail})
	}

	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		Headers("VARIABLE", "ACTION", "WHAT HAPPENS").
		Rows(rows...).
		Render()

	return strings.TrimRight(rendered, "\n")
}
