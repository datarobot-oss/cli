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

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wizard"
)

// credentialSearchLimit bounds the lookup that turns a placeholder into the id
// that belongs there. Past this many credentials the scan is the wrong tool,
// and the entry is still reported as unfinished either way.
const credentialSearchLimit = 200

// verifyCredentials confirms that every credential the manifest references
// actually exists, and is the last check before anything mutates.
//
// A credential reference is the one part of the manifest local validation
// cannot judge. The ledger checks that the shorthand parses; whether the id
// behind it names a real credential is a fact only the platform holds. Left
// unchecked, a mistyped id survives the whole deploy and surfaces as a
// container that will not start, minutes and one paid-for build later, with
// a runtime error that does not mention the manifest.
//
// Every problem is reported at once rather than one per run. The refs come
// from a file the user is about to edit, and finding out about the second bad
// id only after fixing the first is two round trips through a build pipeline
// for what is one edit.
func verifyCredentials(refs []manifest.CredentialRef, workloadName string) error {
	var (
		problems []string
		seen     = make(map[string]bool, len(refs))
	)

	for _, ref := range refs {
		if ref.CredentialID == manifest.CredentialPlaceholder {
			problems = append(problems, placeholderProblem(ref, workloadName))

			continue
		}

		// One GET per credential, not per reference: several variables
		// commonly read different keys of the same stored credential.
		if seen[ref.CredentialID] {
			continue
		}

		seen[ref.CredentialID] = true

		if _, err := getCredentialFn(ref.CredentialID); err != nil {
			if !isNotFound(err) {
				// A transport failure means the references were not checked,
				// which is not the same as their being wrong. Saying so and
				// stopping beats reporting a credential as missing because
				// the platform was briefly unreachable.
				return fmt.Errorf("cannot verify credential %s referenced by %s: %w",
					ref.CredentialID, ref.EnvName, err)
			}

			problems = append(problems, missingProblem(ref))
		}
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%s references credentials that cannot be used:\n  %s",
		manifest.FileName, strings.Join(problems, "\n  "))
}

// placeholderProblem describes a reference the setup wizard wrote and nobody
// finished. The wizard writes the placeholder in full, rather than commenting
// the entry out, precisely so it fails here instead of deploying a container
// without its secret.
func placeholderProblem(ref manifest.CredentialRef, workloadName string) string {
	// The commonest reason setup left a placeholder is that the name it wanted
	// was taken, which means the id the file needs already exists. Saying so
	// with the id in hand turns the message into an edit rather than an
	// errand. A lookup that fails changes nothing: the entry is still wrong.
	name := wizard.CredentialName(workloadName, ref.EnvName)

	if existing, err := findCredentialFn(name, credentialSearchLimit); err == nil && existing != nil {
		return fmt.Sprintf(
			"%s:%d  %s still says %s. A credential named %s already exists, so replacing %s with %s "+
				"is probably what you want, having checked that credential holds this variable's value: "+
				"a name is tenant-wide and says nothing about what is stored under it",
			manifest.FileName, ref.Line, ref.EnvName, manifest.CredentialPlaceholder,
			name, manifest.CredentialPlaceholder, existing.CredentialID)
	}

	return fmt.Sprintf(
		"%s:%d  %s still says %s, so its value would never reach the container. "+
			"Store the value as a credential, then replace %s with that credential's id",
		manifest.FileName, ref.Line, ref.EnvName, manifest.CredentialPlaceholder,
		manifest.CredentialPlaceholder)
}

// missingProblem describes an id the platform does not know. It names the
// variable as well as the id, because the id alone is a hex string the reader
// has to go looking for in the file.
func missingProblem(ref manifest.CredentialRef) string {
	return fmt.Sprintf(
		"line %d: %s references credential %s, which does not exist or is not visible to this account",
		ref.Line, ref.EnvName, ref.CredentialID)
}
