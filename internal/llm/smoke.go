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

package llm

import (
	"context"
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/workload"
)

// Gate is the CLI feature gate for `dr recipe` and the new `dr llm`
// commands while LLM serving is behind the server feature flag.
const Gate = "workload-api-llm"

// Target is where and as what to call an llm workload.
type Target struct {
	WorkloadID string
	Status     string
	Endpoint   string
	ServedName string
}

// ResolveTarget reads the workload's advertised endpoint and its artifact's
// served name, refusing a workload that is not an llm one.
func ResolveTarget(workloadID string) (*Target, error) {
	wl, err := workload.GetWorkload(workloadID)
	if err != nil {
		return nil, explain(err)
	}

	if wl.Type != "" && wl.Type != "llm" {
		return nil, fmt.Errorf("workload %s is a %q workload, not an llm one", workloadID, wl.Type)
	}

	served, err := ServedName(wl.ArtifactID)
	if err != nil {
		return nil, err
	}

	return &Target{WorkloadID: wl.ID, Status: wl.Status, Endpoint: wl.Endpoint, ServedName: served}, nil
}

// SmokeResult says how the post-wait smoke check went.
type SmokeResult struct {
	// Skipped is set, with the reason, when the check could not run from
	// here (an Enclave endpoint); the deploy itself still succeeded.
	Skipped string
}

// Smoke sends one minimal chat request to a running llm workload, so done
// means the model answers at the URL clients use. An error means it did not.
func Smoke(ctx context.Context, workloadID string) (*SmokeResult, error) {
	t, err := ResolveTarget(workloadID) //nolint:contextcheck // the Workload API client takes no context; Chat below honours ctx
	if err != nil {
		return nil, err
	}

	_, err = Chat(ctx, ChatRequest{
		Endpoint:   t.Endpoint,
		ServedName: t.ServedName,
		Messages:   SmokeMessages(),
		MaxTokens:  1,
	}, nil)

	if errors.Is(err, ErrOffHost) {
		return &SmokeResult{Skipped: "the endpoint is on an Enclave, outside this installation"}, nil
	}

	if errors.Is(err, ErrNotIngress) {
		return &SmokeResult{Skipped: "the endpoint is not this installation's public workload ingress, so it is not sent your credentials"}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("the workload is running but the model did not answer a chat request at %s: %w", t.Endpoint, err)
	}

	return &SmokeResult{}, nil
}
