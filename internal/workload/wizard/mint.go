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
	return len(i.Stored) > 0 || len(i.Failed) > 0
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
func importSecrets(vars []manifest.EnvVar, detected Detected, workloadName string) ([]manifest.EnvVar, Import) {
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
func reportImport(stderr io.Writer, report Import) {
	if stderr == nil || !report.Any() {
		return
	}

	if len(report.Stored) > 0 {
		fmt.Fprintf(stderr, "  %s Stored %d %s %s\n",
			tui.SuccessStyle.Render("✓"),
			len(report.Stored), Plural(len(report.Stored), "secret", "secrets"),
			tui.HintStyle.Render("("+JoinNames(report.Stored)+", all in "+manifest.FileName+")"))
	}

	for _, failure := range report.Failed {
		fmt.Fprintf(stderr, "  %s %s\n    %s\n    %s\n",
			tui.WarnStyle.Render("!"),
			tui.WarnStyle.Render(failure.Name+" was not stored"),
			failure.Reason,
			tui.HintStyle.Render(fmt.Sprintf("%s keeps %s for it, and a deploy will refuse that.",
				manifest.FileName, manifest.CredentialPlaceholder)))
	}
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
