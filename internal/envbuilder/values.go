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

func PromptsWithValues(prompts []UserPrompt, variables Variables) []UserPrompt {
	for p, prompt := range prompts {
		// Capture existing env var values
		existingEnvValue, ok := os.LookupEnv(prompt.Env)
		if ok {
			prompt.Value = existingEnvValue
		} else if v, found := variables.find(prompt.Env); found {
			prompt.Value = v.Value
			prompt.Commented = v.Commented
		} else {
			prompt.Value = prompt.Default
		}

		prompts[p] = prompt
	}

	return prompts
}

func (vv Variables) find(name string) (Variable, bool) {
	currentVariableIndex := slices.IndexFunc(vv, func(v Variable) bool {
		return v.Name == name
	})

	if currentVariableIndex == -1 {
		return Variable{}, false
	}

	return vv[currentVariableIndex], true
}
