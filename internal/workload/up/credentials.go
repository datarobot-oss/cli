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
)

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
func verifyCredentials(refs []manifest.CredentialRef) error {
	var (
		problems []string
		seen     = make(map[string]bool, len(refs))
	)

	for _, ref := range refs {
		if ref.CredentialID == manifest.CredentialPlaceholder {
			problems = append(problems, placeholderProblem(ref))

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

	return fmt.Errorf("the manifest references credentials that cannot be used:\n  %s",
		strings.Join(problems, "\n  "))
}

// placeholderProblem describes a reference the setup wizard wrote and nobody
// finished. The wizard writes the placeholder in full, rather than commenting
// the entry out, precisely so it fails here instead of deploying a container
// without its secret.
func placeholderProblem(ref manifest.CredentialRef) string {
	return fmt.Sprintf(
		"line %d: %s still references the %s written by setup. "+
			"Create the credential, then replace %s with its id",
		ref.Line, ref.EnvName, manifest.CredentialPlaceholder, manifest.CredentialPlaceholder)
}

// missingProblem describes an id the platform does not know. It names the
// variable as well as the id, because the id alone is a hex string the reader
// has to go looking for in the file.
func missingProblem(ref manifest.CredentialRef) string {
	return fmt.Sprintf(
		"line %d: %s references credential %s, which does not exist or is not visible to this account",
		ref.Line, ref.EnvName, ref.CredentialID)
}
