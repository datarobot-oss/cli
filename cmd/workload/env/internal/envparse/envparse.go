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

// Package envparse holds the NAME=VALUE / credential-reference parsing and
// validation shared by `dr workload env set` and `dr workload env import` --
// both need to turn user-supplied strings into workload.EnvironmentVar
// values with identical rules, so the logic lives here once rather than
// drifting between two copies.
package envparse

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
)

// CredentialPrefix marks a VALUE as a reference to a stored DataRobot
// credential rather than a literal string, in the form
// dr-credential:<credential-id>/<credential-key>. "credential-key" picks
// which field of the credential to use -- a single stored credential can
// bundle several secret fields (an S3 credential has awsAccessKeyId,
// awsSecretAccessKey, and awsSessionToken, for instance), so this is not the
// same "key" as the KEY in KEY=VALUE. See the datarobot-workload-api skill's
// schema-reference.md for the credential type -> valid credential-key table.
const CredentialPrefix = "dr-credential:"

// envVarNamePattern is Kubernetes' own container env var name rule
// (IsEnvVarName in k8s.io/apimachinery/pkg/util/validation): starts with a
// letter, underscore, or dot, followed by letters, digits, underscores,
// dots, or dashes. Verified live against staging that the platform accepts
// -- and silently writes -- names violating this (spaces, a leading digit,
// dashes) at PATCH time with no complaint; rejecting here catches a typo
// immediately instead of it surfacing later as a replacement failure or a
// crash-looping container with no obvious link back to this command.
var envVarNamePattern = regexp.MustCompile(`^[-._a-zA-Z][-._a-zA-Z0-9]*$`)

// ParseArg splits a KEY=VALUE argument, recognizing the
// dr-credential:<credential-id>/<credential-key> value form as a
// credential-backed var and everything else as a plain literal. Used for
// `env set`'s positional NAME=VALUE arguments.
func ParseArg(arg string) (workload.EnvironmentVar, error) {
	name, value, ok := strings.Cut(arg, "=")
	if !ok || name == "" {
		return workload.EnvironmentVar{}, fmt.Errorf("invalid argument %q: expected KEY=VALUE", arg)
	}

	return BuildVar(name, value)
}

// BuildVar validates name and turns (name, value) into a
// workload.EnvironmentVar, recognizing the dr-credential:<id>/<key> value
// form. Used directly by `env import`, whose file parser already splits
// each line into a name and a value.
func BuildVar(name, value string) (workload.EnvironmentVar, error) {
	if !envVarNamePattern.MatchString(name) {
		return workload.EnvironmentVar{}, fmt.Errorf(
			"invalid environment variable name %q: must consist of letters, digits, '_', '-', or '.', and must not start with a digit",
			name)
	}

	credRef, isCredential := strings.CutPrefix(value, CredentialPrefix)
	if !isCredential {
		return workload.EnvironmentVar{Name: name, Value: value}, nil
	}

	credentialID, credentialKey, ok := strings.Cut(credRef, "/")
	if !ok || credentialID == "" || credentialKey == "" {
		return workload.EnvironmentVar{}, fmt.Errorf(
			"invalid credential reference %q: expected %s<credential-id>/<credential-key>", value, CredentialPrefix)
	}

	return workload.EnvironmentVar{
		Source:         workload.EnvironmentVarSourceDRCredential,
		Name:           name,
		DRCredentialID: credentialID,
		Key:            credentialKey,
	}, nil
}

// ValidateCredentialReferences confirms every credential-backed var in vars
// references a credential id that actually exists, so a typo'd or
// copy-pasted-wrong id is caught here rather than surfacing later as a
// replacement failure or a crash-looping container with no obvious link
// back to this command (verified live: the platform accepts and silently
// writes a nonexistent credential id at PATCH time). Deliberately does not
// validate that Key is one of the credential's actual field names -- the
// credential-type -> valid-key mapping is a hand-maintained table that can
// drift from the platform's real schema, so a false rejection there would
// be worse than leaving it to the platform's own validation. Repeated
// credential ids across multiple vars are only checked once.
func ValidateCredentialReferences(vars []workload.EnvironmentVar) error {
	checked := make(map[string]bool)

	var missing []string

	for _, v := range vars {
		if v.Source != workload.EnvironmentVarSourceDRCredential || checked[v.DRCredentialID] {
			continue
		}

		checked[v.DRCredentialID] = true

		if _, err := workload.GetCredential(v.DRCredentialID); err != nil {
			var httpErr *drapi.HTTPError

			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
				missing = append(missing, v.DRCredentialID)

				continue
			}

			return fmt.Errorf("check credential %s: %w", v.DRCredentialID, err)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("credential id(s) not found: %s", strings.Join(missing, ", "))
	}

	return nil
}
