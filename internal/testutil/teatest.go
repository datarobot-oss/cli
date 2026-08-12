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

package testutil

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

const (
	// finalModelTimeout bounds how long FinalModel waits for teatest to publish
	// the model after the program has reported that it finished.
	finalModelTimeout = 5 * time.Second
	// finalModelPollInterval is how often FinalModel re-checks in that window.
	finalModelPollInterval = time.Millisecond
)

// FinalModel returns the model a teatest program ended with, retrying until
// teatest has actually published it.
//
// teatest.TestModel runs the program on its own goroutine and, when it returns,
// signals completion on doneCh *before* sending the final model on modelCh:
//
//	tm.doneCh <- true
//	tm.modelCh <- m
//
// teatest's own TestModel.FinalModel waits on doneCh and then does a
// *non-blocking* receive on modelCh, falling back to tm.model — which is never
// assigned before the program finishes. A caller that lands between those two
// sends therefore gets a nil model back, and any type assertion on it fails
// ("final model is not of type X"). The window is tiny, so this only bites
// under load: a shuffled or heavily parallel CI run.
//
// Calling TestModel.FinalModel again closes the gap, since its wait is
// sync.Once-guarded and the second receive picks up the published model.
func FinalModel(t *testing.T, tm *teatest.TestModel, opts ...teatest.FinalOpt) tea.Model {
	t.Helper()

	deadline := time.Now().Add(finalModelTimeout)

	for {
		if model := tm.FinalModel(t, opts...); model != nil {
			return model
		}

		if time.Now().After(deadline) {
			t.Fatalf("teatest did not publish a final model within %s", finalModelTimeout)

			return nil
		}

		time.Sleep(finalModelPollInterval)
	}
}
