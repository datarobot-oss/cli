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
	"io"
	"net/http"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// Import is what storing the secrets did, one entry per secret the run tried
// to store. It exists so the caller can report the outcome per variable: a
// partial import is the normal failure here, not an exceptional one, and
// "3 of 5 stored" is the only honest way to say it.
type Import struct {
	// Stored are the variables now carrying a credential id.
	Stored []string
	// Reused are the variables that pointed at a credential already holding
	// their name, whose value was replaced with the one in .env on the user's
	// say-so.
	Reused []string
	// Failed are the variables still carrying the placeholder, each with the
	// reason, ready to be printed.
	Failed []ImportFailure
}

// ImportFailure is one secret that did not reach the credential store.
type ImportFailure struct {
	// Name is the environment variable, which is what the user recognises;
	// the credential name is derived from it and is noise by comparison.
	Name string
	// Reason is why, phrased for someone who now has to finish the job by
	// hand. It never carries the value.
	Reason string
}

// Any reports whether anything was tried at all.
func (i Import) Any() bool {
	return len(i.Stored) > 0 || len(i.Reused) > 0 || len(i.Failed) > 0
}

// importSecrets stores each secret and writes the credential id back into the
// variable, so the manifest can reference it instead of carrying a
// placeholder. It is the half of the .env import that needs the network, and
// the only place in setup that sends a secret anywhere.
//
// Values come from detected rather than from vars: a manifest.EnvVar has no
// value for a secret by construction, which is what keeps a secret out of the
// file even when a render goes wrong. The value is read here, sent, and
// dropped.
//
// One secret failing does not fail the run. The variable keeps its
// placeholder, which is an entry a deploy refuses by name, so the worst case
// is the state this whole feature started from rather than a wizard that
// leaves nothing on disk after asking eight questions.
//
// A name already taken is the one failure with a way forward: the credential
// that holds it is looked up, and reuse (nil means never) is asked whether to
// point the variable at it and replace its value with the one in .env.
func importSecrets(
	vars []manifest.EnvVar, detected Detected, workloadName string, reuse ReuseFunc,
) ([]manifest.EnvVar, Import) {
	values := secretValues(detected)
	out := make([]manifest.EnvVar, len(vars))
	copy(out, vars)

	var report Import

	for i, v := range out {
		if !v.Secret || v.CredentialID != "" {
			continue
		}

		value, ok := values[v.Name]
		if !ok || value == "" {
			// Classified as a secret with nothing to store: an empty
			// assignment in the .env, or a name the table added by hand.
			report.Failed = append(report.Failed, ImportFailure{
				Name:   v.Name,
				Reason: "it has no value in " + EnvFileName,
			})

			continue
		}

		name := CredentialName(workloadName, v.Name)

		cred, err := createCredentialFn(name, value)
		if isNameTaken(err) {
			reused, reason := reuseTaken(v.Name, name, value, reuse)
			if reused == nil {
				report.Failed = append(report.Failed, ImportFailure{Name: v.Name, Reason: reason})

				continue
			}

			out[i].CredentialID = reused.CredentialID

			report.Reused = append(report.Reused, v.Name)

			continue
		}

		if err != nil {
			report.Failed = append(report.Failed, ImportFailure{Name: v.Name, Reason: importReason(err, name)})

			continue
		}

		out[i].CredentialID = cred.CredentialID

		report.Stored = append(report.Stored, v.Name)
	}

	return out, report
}

// secretValues indexes the detected values by name. The map is built once
// rather than searched per variable, and holds only what is about to be sent.
func secretValues(detected Detected) map[string]string {
	values := make(map[string]string, len(detected.EnvVars))

	for _, v := range detected.EnvVars {
		values[v.Name] = v.Value
	}

	return values
}

// CredentialName is what the credential is called in a store shared by the
// whole tenant. The workload name is the prefix because a bare OPENAI_API_KEY
// belongs to whoever created it first, and the second project to try would
// collide with a credential it cannot see the value of.
//
// Exported because the deploy needs it too: a manifest still carrying the
// placeholder is one lookup away from the id that belongs there.
func CredentialName(workloadName, envName string) string {
	if workloadName == "" {
		return envName
	}

	return workloadName + "/" + envName
}

// ReuseFunc is asked, once per variable, whether to point it at the credential
// that already holds its name and replace that credential's value with the one
// in .env. nil means no: --yes agrees to the import, not to overwriting a
// credential somebody may be relying on.
type ReuseFunc func(envName, credentialName, credentialID string) bool

// credentialSearchLimit bounds the lookup of the credential holding a taken
// name, the same bound the deploy uses for the same question.
const credentialSearchLimit = 200

// findCredentialFn is the seam for turning a taken name into the credential
// that holds it.
var findCredentialFn = workload.FindCredentialNamed

func isNameTaken(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict
}

// reuseTaken settles a credential name that is already in use. It returns the
// credential to use, or nil with the reason it could not be, phrased so the
// reader can finish by hand.
//
// Only an API-token credential is ever overwritten: that is the one shape this
// CLI writes, and the store is tenant-wide, so a same-named credential of
// another type belongs to somebody else's use.
func reuseTaken(envName, name, value string, reuse ReuseFunc) (*workload.Credential, string) {
	existing, err := findCredentialFn(name, credentialSearchLimit)
	if err != nil || existing == nil {
		return nil, takenReason(name, nil)
	}

	if existing.CredentialType != workload.CredentialTypeAPIToken {
		return nil, fmt.Sprintf(
			"a credential called %q (%s) already exists, but it is not an API-token credential, "+
				"so it is not overwritten. Deploy under a different workload name, or point the entry at a "+
				"credential you have checked holds this value.", name, existing.CredentialID)
	}

	if reuse == nil || !reuse(envName, name, existing.CredentialID) {
		return nil, takenReason(name, existing)
	}

	if _, err := updateCredentialFn(existing.CredentialID, value); err != nil {
		return nil, fmt.Sprintf("credential %q (%s) exists, but its value could not be replaced: %s",
			name, existing.CredentialID, strings.TrimSpace(err.Error()))
	}

	return existing, ""
}

// takenReason says what to do about a name that is in use. With the holder
// found it names the id and the exact line to write, because a message that
// says "paste its id" without the id sends the reader looking for it.
func takenReason(name string, existing *workload.Credential) string {
	if existing == nil {
		return fmt.Sprintf(
			"a credential called %q already exists in your organisation, and its value may not be this one. "+
				"Find its id, check it holds this value, and put it in %s, or deploy under a different workload name.",
			name, manifest.FileName)
	}

	return fmt.Sprintf(
		"a credential called %q already exists in your organisation (id %s), and its value may not be this one.\n"+
			"    To use it as it is, set the entry's value to %s%s/%s in %s.\n"+
			"    To replace its value with the one in %s, run this again where it can ask you.",
		name, existing.CredentialID,
		manifest.CredentialShorthandPrefix, existing.CredentialID, manifest.CredentialKeyWritten, manifest.FileName,
		EnvFileName)
}

// importReason turns a failure into something worth reading. A name already
// taken is the common one: credential names are unique across the whole
// organisation, so setting up a second project under the same workload name
// reaches it every time. The value behind that name is not visible from here
// and may not be the one in this .env, which is why it is not reused silently.
func importReason(err error, name string) string {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
		return fmt.Sprintf(
			"a credential called %q already exists in your organisation, and its value may not be this one. "+
				"Reuse it by pasting its id, or deploy under a different workload name.", name)
	}

	return strings.TrimSpace(err.Error())
}

// reportImport says what reached the credential store. Silence would be the
// wrong answer either way: a secret that was stored is a thing that now exists
// outside this repository, and one that was not is an entry the next deploy
// will refuse.
func reportImport(stderr io.Writer, report Import, keepsPlaceholder func(name string) bool) {
	if stderr == nil || !report.Any() {
		return
	}

	if len(report.Stored) > 0 {
		fmt.Fprintf(stderr, "  %s Stored %d %s %s\n",
			tui.SuccessStyle.Render("✓"),
			len(report.Stored), Plural(len(report.Stored), "secret", "secrets"),
			tui.HintStyle.Render("("+JoinNames(report.Stored)+", all in "+manifest.FileName+")"))
	}

	if len(report.Reused) > 0 {
		fmt.Fprintf(stderr, "  %s Reused %d existing %s, with the value from %s %s\n",
			tui.SuccessStyle.Render("✓"),
			len(report.Reused), Plural(len(report.Reused), "credential", "credentials"), EnvFileName,
			tui.HintStyle.Render("("+JoinNames(report.Reused)+")"))
	}

	for _, failure := range report.Failed {
		fmt.Fprintf(stderr, "  %s %s\n    %s\n    %s\n",
			tui.WarnStyle.Render("!"),
			tui.WarnStyle.Render(failure.Name+" was not stored"),
			failure.Reason,
			tui.HintStyle.Render(unstoredOutcome(keepsPlaceholder == nil || keepsPlaceholder(failure.Name))))
	}
}

// unstoredOutcome is what the file is left holding for a secret that never
// reached the store. The two answers are different enough that saying the
// wrong one is worse than saying nothing.
//
// A name being added has nowhere to point, so its new entry carries the
// placeholder and the next deploy refuses it. A name the manifest already
// declares as something else keeps exactly what it had: replacing a working
// literal with a placeholder would throw the value away and break a deploy
// that was working, on the strength of a store that could not be reached.
func unstoredOutcome(keepsPlaceholder bool) string {
	if keepsPlaceholder {
		return fmt.Sprintf("%s keeps %s for it, and a deploy will refuse that.",
			manifest.FileName, manifest.CredentialPlaceholder)
	}

	return manifest.FileName + " keeps the entry it already had, so nothing is lost and nothing is reconciled for it."
}

// pendingSecrets counts the entries still carrying a placeholder, which is
// what the command reports as unfinished.
func pendingSecrets(vars []manifest.EnvVar) int {
	pending := 0

	for _, v := range vars {
		if v.Secret && v.CredentialID == "" {
			pending++
		}
	}

	return pending
}

// createCredentialFn is the seam the tests replace: storing a secret is the
// one call in setup that cannot be allowed to reach a real tenant by accident.
var createCredentialFn = workload.CreateCredential

// updateCredentialFn re-sends a value to a credential that already exists,
// which is the only route a rotated .env secret has to a running workload.
var updateCredentialFn = workload.UpdateCredential

// getCredentialFn reads a credential's metadata, which is how a rotation
// checks that the id in the manifest names one this project created.
var getCredentialFn = workload.GetCredential

// reportRotation says what re-sending the secrets did. It is separate from
// reportImport because the acts differ in what they risk: an import that fails
// leaves a placeholder a deploy refuses by name, while a rotation that fails
// leaves the previous value in place and the container serving it, which
// nothing downstream can notice.
func reportRotation(stderr io.Writer, rotated int, failed []ImportFailure, dryRun bool) {
	if stderr == nil || (rotated == 0 && len(failed) == 0) {
		return
	}

	if rotated > 0 {
		fmt.Fprint(stderr, rotationLine(rotated, dryRun))
	}

	for _, failure := range failed {
		fmt.Fprintf(stderr, "  %s %s was not re-sent: %s. The workload keeps the value it has.\n",
			tui.WarnStyle.Render("!"), failure.Name, failure.Reason)
	}
}

// rotationLine reports the sends, in the tense the run earned: a dry run says
// what it would do, and only a run that reached the store gets the tick.
func rotationLine(rotated int, dryRun bool) string {
	if dryRun {
		return fmt.Sprintf("  %d %s would be re-sent to the credential store.\n",
			rotated, Plural(rotated, "secret", "secrets"))
	}

	return fmt.Sprintf("  %s Re-sent %d %s to the credential store.\n",
		tui.SuccessStyle.Render("✓"), rotated, Plural(rotated, "secret", "secrets"))
}
