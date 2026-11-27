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
	"errors"
	"net/http"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
)

// FeatureFlag is the user flag that opens recipes and llm artifacts. The
// Workload API reads it from the user's account/info permissions, so it
// needs an explicit user, organization or group grant: the installation's
// default alone does not open it.
const FeatureFlag = "ENABLE_WORKLOAD_API_TOKEN_FACTORY"

// The Workload API's details for the two gates, matched on a stable phrase
// rather than the whole sentence so wording tweaks do not break the mapping.
const (
	clusterDisabledPhrase = "disabled on this cluster"
	userFlagPhrase        = "not available for the current user"

	// Workload Storage refusals, for a deploy with a model weight mount. Checked
	// first: "Workload Storage is disabled on this cluster" also contains the
	// LLM serving phrase.
	storagePhrase         = "workload storage"
	storageUserFlagPhrase = "not enabled for this user"
	storageFeatureFlag    = "ENABLE_WORKLOAD_API_STORAGE"
)

// Remedies shown for the two gates.
const (
	ClusterDisabledRemedy = "LLM serving is not enabled on this installation. Ask your administrator to enable it."
	UserFlagRemedy        = "LLM serving needs the " + FeatureFlag + " feature flag granted to your user, organization or group. Ask an administrator."

	StorageClusterDisabledRemedy = "A model weight storage needs Workload Storage, which is not enabled on this installation. Ask your administrator to enable it, or deploy without --model-weight-size/--model-weight-storage-id."
	StorageUserFlagRemedy        = "A model weight storage needs the " + storageFeatureFlag + " feature flag granted to your user, organization or group. Ask an administrator."
)

// GateError is a 403 from one of the two LLM serving gates. It keeps the
// original error so errors.As still finds the *drapi.HTTPError.
type GateError struct {
	Remedy string
	Err    error
}

func (e *GateError) Error() string {
	return e.Remedy + "\n" + e.Err.Error()
}

func (e *GateError) Unwrap() error {
	return e.Err
}

func explainGate(err error) error {
	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusForbidden {
		return err
	}

	detail := strings.ToLower(httpErr.Detail)

	switch {
	case strings.Contains(detail, storagePhrase) && strings.Contains(detail, storageUserFlagPhrase):
		return &GateError{Remedy: StorageUserFlagRemedy, Err: err}
	case strings.Contains(detail, storagePhrase) && strings.Contains(detail, clusterDisabledPhrase):
		return &GateError{Remedy: StorageClusterDisabledRemedy, Err: err}
	case strings.Contains(detail, clusterDisabledPhrase):
		return &GateError{Remedy: ClusterDisabledRemedy, Err: err}
	case strings.Contains(detail, userFlagPhrase):
		return &GateError{Remedy: UserFlagRemedy, Err: err}
	default:
		return err
	}
}
