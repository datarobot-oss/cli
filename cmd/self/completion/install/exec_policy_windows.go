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

//go:build windows

package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const policyWarning = `  ⚠  PowerShell execution policy is set to %s.
     The completion profile was written but will not load on the next
     PowerShell launch.

     Run this command to fix it:
       Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
`

func warnIfExecutionPolicyRestricted(shellExe string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, shellExe, "-Command", "Get-ExecutionPolicy").Output()
	if err != nil {
		return
	}

	policy := strings.TrimSpace(string(out))

	if policy == "Restricted" {
		fmt.Fprintf(os.Stderr, policyWarning, policy)
	}
}
