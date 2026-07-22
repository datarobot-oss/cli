//go:build windows

package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const policyWarning = `  ⚠  PowerShell execution policy is set to %s.
     The completion profile was written but will not load on the next
     PowerShell launch.

     Run this command to fix it:
       Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
`

func warnIfExecutionPolicyRestricted() {
	out, err := exec.Command("powershell", "-Command", "Get-ExecutionPolicy -Scope CurrentUser").Output()
	if err != nil {
		return
	}

	policy := strings.TrimSpace(string(out))

	if policy == "Restricted" || policy == "Undefined" {
		fmt.Fprintf(os.Stderr, policyWarning, policy)
	}
}
