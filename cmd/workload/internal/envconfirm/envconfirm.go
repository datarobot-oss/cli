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

// Package envconfirm decides who may agree to a .env reconciliation, and asks
// them, for the two commands that offer one.
//
// It lives here rather than in either command because both ask the same
// question about the same two files, and a prompt that differed between them
// would make the answer mean two things.
package envconfirm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/datarobot/cli/tui"
)

// Policy is what a run is allowed to do about the question, which is a
// different question from what it is allowed to do about a prompt.
//
// A sync writes to two places. One is a file under version control, which is
// recoverable. The other is the tenant's credential store, where the value
// sent is whatever the local copy of .env happens to hold: a stale key on a
// laptop overwrites a good one that other runs of this workload depend on, and
// nothing on the platform records that it happened. That is the act this
// governs.
type Policy struct {
	// Yes is the --yes flag, and only the flag. It is consent, and it is the
	// one thing that is.
	Yes bool

	// DryRun changes neither file nor store, so there is nothing to consent
	// to and nobody needs asking. The table still prints: previewing the sync
	// is the whole of what the run was for.
	DryRun bool

	// Interactive says there is a terminal to put the question on.
	Interactive bool
}

// Ask returns the function `wizard.Options.Confirm` expects.
//
// nil means go ahead without asking, which is --yes and a dry run.
//
// Without a terminal and without --yes it returns a refusal rather than
// consent. A pipeline that reached this without saying yes has not agreed to
// anything: `dr workload delete` already draws the same line, on the grounds
// that the environment variable which suppresses wizards in CI is not consent
// to do something irreversible, and DATAROBOT_CLI_NON_INTERACTIVE is
// deliberately not read here for that reason. Silently applying instead would
// make a scheduled deploy the one caller that overwrites tenant secrets with
// no record of having been allowed to.
func Ask(stderr io.Writer, stdin io.Reader, policy Policy) func() (bool, error) {
	if policy.Yes || policy.DryRun {
		return nil
	}

	if !policy.Interactive {
		return refuse
	}

	return func() (bool, error) {
		fmt.Fprintf(stderr, "\n  %s\n", tui.HintStyle.Render(
			"A rewrite lands in a file you commit, and a re-send overwrites the value on the tenant."))
		fmt.Fprint(stderr, "\n  Apply this? [y/N] ")

		// One line, like the deploy's own confirmation. An answer that cannot
		// be read at all is a no, which leaves both files as they are.
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return false, nil
		}

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
}

// refuse is what a run with nobody to ask gets. An error rather than a quiet
// skip, because the flag was passed on purpose and a run that ignored it would
// report a reconciliation it never carried out.
func refuse() (bool, error) {
	return false, errors.New(
		"--sync-env rewrites a file you commit and sends values to the credential store, and there is no " +
			"terminal here to confirm that on. Add --yes to say so explicitly, or run it where you can answer. " +
			"The table above is what it would have done")
}
