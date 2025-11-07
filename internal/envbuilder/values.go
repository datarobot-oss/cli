// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"os"
	"slices"
)

func promptsWithValues(prompts []UserPrompt, variables Variables) []UserPrompt {
	if len(variables) == 0 {
		return prompts
	}

	for p, prompt := range prompts {
		// Capture existing env var values
		if existingEnvValue, ok := os.LookupEnv(prompt.Env); ok {
			prompt.Value = existingEnvValue
		} else if v, found := variables.find(prompt); found {
			prompt.Value = v.Value
			prompt.Commented = v.Commented
		} else {
			prompt.Value = prompt.Default
		}

		prompts[p] = prompt
	}

	return prompts
}

func indexByName(value string) func(v Variable) bool {
	return func(v Variable) bool {
		return v.Name == value
	}
}

func (vv Variables) find(prompt UserPrompt) (Variable, bool) {
	if envIndex := slices.IndexFunc(vv, indexByName(prompt.Env)); envIndex != -1 {
		return vv[envIndex], true
	}

	if keyIndex := slices.IndexFunc(vv, indexByName(prompt.Key)); keyIndex != -1 {
		return vv[keyIndex], true
	}

	return Variable{}, false
}

func (vv Variables) valuesMap() map[string]string {
	envValues := make(map[string]string)

	for _, v := range vv {
		if v.Name != "" && !v.Commented {
			envValues[v.Name] = v.Value
		}
	}

	return envValues
}
