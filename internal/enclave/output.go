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

package enclave

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

const emptyValuePlaceholder = "—"

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// AccessChange is the stable JSON shape emitted after a grant/revoke, and the
// data backing the text confirmation line.
type AccessChange struct {
	EnclaveID     string `json:"enclaveId"`
	Action        string `json:"action"`         // granted | revoked
	Role          string `json:"role,omitempty"` // set for grant only
	RecipientType string `json:"recipientType"`
	Recipient     string `json:"recipient"`
}

// RenderAccessChange prints the outcome of a grant/revoke. Text mode prints a
// one-line confirmation; JSON mode emits the AccessChange document.
func RenderAccessChange(format outputformat.OutputFormat, change AccessChange) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(change)
	}

	recipient := change.RecipientType + " " + change.Recipient

	if change.Action == "granted" {
		fmt.Printf("granted %s on enclave %s to %s\n", change.Role, change.EnclaveID, recipient)
	} else {
		fmt.Printf("revoked access on enclave %s from %s\n", change.EnclaveID, recipient)
	}

	return nil
}

// PermissionChange is the stable JSON shape emitted after a collection-level
// permission grant/revoke, and the data backing the text confirmation line.
type PermissionChange struct {
	Permission    string `json:"permission"`
	Action        string `json:"action"` // granted | revoked
	RecipientType string `json:"recipientType"`
	Recipient     string `json:"recipient"`
}

// RenderPermissionChange prints the outcome of a permission grant/revoke. Text
// mode prints a one-line confirmation; JSON mode emits the PermissionChange
// document. There is no enclave id — the permission spans the collection.
func RenderPermissionChange(format outputformat.OutputFormat, change PermissionChange) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(change)
	}

	recipient := change.RecipientType + " " + change.Recipient

	if change.Action == "granted" {
		fmt.Printf("granted enclave %s permission to %s\n", change.Permission, recipient)
	} else {
		fmt.Printf("revoked enclave %s permission from %s\n", change.Permission, recipient)
	}

	return nil
}

// RenderEnclavePermissions prints the effective permissions on an enclave. Text
// mode explains *why* the set is what it is, since a full set from the sys-admin
// bypass or from RBAC being off looks identical to being an owner.
func RenderEnclavePermissions(format outputformat.OutputFormat, p EnclavePermissions) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(p)
	}

	fmt.Printf("Enclave:     %s\n", p.EnclaveID)
	fmt.Printf("Subject:     %s\n", p.SubjectUserID)

	if len(p.Permissions) == 0 {
		fmt.Printf("Permissions: %s (no access)\n", emptyValuePlaceholder)
	} else {
		fmt.Printf("Permissions: %s\n", strings.Join(p.Permissions, ", "))
	}

	switch {
	case !p.RBACEnabled:
		fmt.Println("Source:      enclave RBAC is disabled server-side — nothing is enforced")
	case p.ViaSysAdmin:
		fmt.Println("Source:      system administrator (bypasses enclave permissions)")
	default:
		fmt.Println("Source:      granted permissions")
	}

	// Only worth printing when it differs from the effective set: under a bypass the
	// effective set is always full, so this is the only way to see whether the
	// subject was granted anything at all.
	if p.RBACEnabled && p.ViaSysAdmin {
		if len(p.GrantedPermissions) == 0 {
			fmt.Printf("Granted:     %s (no grants of its own)\n", emptyValuePlaceholder)
		} else {
			fmt.Printf("Granted:     %s\n", strings.Join(p.GrantedPermissions, ", "))
		}
	}

	return nil
}

// RenderSharedRoles prints who holds which role on an enclave.
func RenderSharedRoles(format outputformat.OutputFormat, roles []SharedRole) error {
	if format == outputformat.OutputFormatJSON {
		if roles == nil {
			roles = []SharedRole{}
		}

		return outputformat.PrintJSONEnvelope(os.Stdout, "sharedRoles", roles)
	}

	if len(roles) == 0 {
		fmt.Println("This enclave is not shared with anyone.")

		return nil
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			return cellStyle
		}).
		Headers("RECIPIENT TYPE", "NAME", "ID", "ROLE")

	for _, r := range roles {
		name := r.Name
		if name == "" {
			name = emptyValuePlaceholder
		}

		t.Row(r.ShareRecipientType, name, r.ID, r.Role)
	}

	fmt.Fprintln(os.Stdout, t.Render())

	return nil
}

// RenderEnclave renders a single enclave (get / deactivate result).
func RenderEnclave(format outputformat.OutputFormat, e Enclave) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(e)
	}

	printEnclaveDetails(e)

	return nil
}

// RenderEnclaves renders the list result as a table (text) or JSON envelope.
func RenderEnclaves(format outputformat.OutputFormat, enclaves []Enclave) error {
	if format == outputformat.OutputFormatJSON {
		if enclaves == nil {
			enclaves = []Enclave{}
		}

		return outputformat.PrintJSONEnvelope(os.Stdout, "enclaves", enclaves)
	}

	printEnclavesTable(enclaves)

	return nil
}

// RenderRegistration renders the register result. Text mode prints the created
// enclave plus its one-shot installation secrets with a warning that they are
// shown only once; JSON mode emits the whole document so scripts keep the
// secrets. Because the server never persists the secrets, this output is the
// only place they can be captured.
func RenderRegistration(format outputformat.OutputFormat, result RegistrationResult) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(result)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", result.EnclaveID)
	fmt.Fprintf(w, "Name:\t%s\n", result.Name)
	fmt.Fprintf(w, "Status:\t%s\n", result.Status)

	w.Flush()

	printInstallationSecrets(result.InstallationSecrets)

	return nil
}

func printEnclaveDetails(e Enclave) {
	dispatch := e.DispatchURL
	if dispatch == "" {
		dispatch = emptyValuePlaceholder
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", e.ID)
	fmt.Fprintf(w, "Name:\t%s\n", e.Name)
	fmt.Fprintf(w, "Status:\t%s\n", e.Status)
	fmt.Fprintf(w, "Dispatch URL:\t%s\n", dispatch)

	w.Flush()
}

func printEnclavesTable(enclaves []Enclave) {
	if len(enclaves) == 0 {
		fmt.Println("No enclaves found.")

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	headers := []string{"ENCLAVE ID", "NAME", "STATUS", "DISPATCH URL"}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, e := range enclaves {
		dispatch := e.DispatchURL
		if dispatch == "" {
			dispatch = emptyValuePlaceholder
		}

		t.Row(e.ID, e.Name, e.Status, dispatch)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// printInstallationSecrets renders each returned secret and its key/value data
// under a shown-once warning, followed by a kubectl hint for materializing it.
func printInstallationSecrets(secrets map[string]InstallationSecret) {
	if len(secrets) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Bold(true).Render("Installation secrets (shown once — save them now):"))

	names := make([]string, 0, len(secrets))

	for name := range secrets {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		fmt.Printf("\n  %s\n", tui.BaseTextStyle.Bold(true).Render(name))

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		keys := make([]string, 0, len(secrets[name].Data))

		for k := range secrets[name].Data {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(w, "    %s:\t%s\n", k, secrets[name].Data[k])
		}

		w.Flush()

		fmt.Println(tui.DimStyle.Render(
			"    Create on the enclave cluster with:\n" +
				"      kubectl create secret generic " + name + " \\\n" +
				"        " + kubectlLiterals(secrets[name].Data)))
	}
}

// kubectlLiterals renders the --from-literal flags for a secret's data in a
// stable key order, joined for a single-line continuation.
func kubectlLiterals(data map[string]string) string {
	keys := make([]string, 0, len(data))

	for k := range data {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	out := ""

	for i, k := range keys {
		if i > 0 {
			out += " "
		}

		out += fmt.Sprintf("--from-literal=%s=%s", k, data[k])
	}

	return out
}

// RenderCollectionPermissions prints whether the subject may create enclaves, and
// where that comes from.
func RenderCollectionPermissions(
	format outputformat.OutputFormat, p CollectionPermissions,
) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(p)
	}

	fmt.Printf("Subject:     %s\n", p.SubjectUserID)

	if len(p.Permissions) == 0 {
		fmt.Printf("Permissions: %s (may not create enclaves)\n", emptyValuePlaceholder)
	} else {
		fmt.Printf("Permissions: %s\n", strings.Join(p.Permissions, ", "))
	}

	switch {
	case !p.RBACEnabled:
		fmt.Println("Source:      enclave RBAC is disabled server-side — nothing is enforced")
	case p.ViaSysAdmin:
		fmt.Println("Source:      system administrator (bypasses enclave permissions)")
	default:
		fmt.Println("Source:      granted permissions")
	}

	if p.RBACEnabled && p.ViaSysAdmin {
		if len(p.GrantedPermissions) == 0 {
			fmt.Printf("Granted:     %s (no grants of its own)\n", emptyValuePlaceholder)
		} else {
			fmt.Printf("Granted:     %s\n", strings.Join(p.GrantedPermissions, ", "))
		}
	}

	return nil
}

// RenderCreateAccess prints who may create enclaves.
func RenderCreateAccess(format outputformat.OutputFormat, holders []CreateAccessHolder) error {
	if format == outputformat.OutputFormatJSON {
		if holders == nil {
			holders = []CreateAccessHolder{}
		}

		return outputformat.PrintJSONEnvelope(os.Stdout, "createAccess", holders)
	}

	if len(holders) == 0 {
		fmt.Println("Nobody has been granted the enclave create permission.")

		return nil
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			return cellStyle
		}).
		Headers("RECIPIENT TYPE", "ID", "PERMISSIONS")

	for _, h := range holders {
		t.Row(h.ShareRecipientType, h.ID, strings.Join(h.Permissions, ", "))
	}

	fmt.Fprintln(os.Stdout, t.Render())

	return nil
}
