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

package cmd

import (
	"fmt"
	"io"
)

// fprintNoWindowsCertsHelp explains what to do when --export-windows-certs finds an
// empty certificate store. The export itself worked; there is simply nothing to
// export, which is a machine-configuration problem the user can resolve.
//
// Kept build-tag free so it can be unit tested on any host, although only the
// Windows build calls it.
func fprintNoWindowsCertsHelp(w io.Writer) {
	fmt.Fprintln(w, "The Windows certificate store contains no Root or CA certificates to export.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Check what is installed:")
	fmt.Fprintln(w, "    Get-ChildItem Cert:\\LocalMachine\\Root")
	fmt.Fprintln(w, "    Get-ChildItem Cert:\\CurrentUser\\Root")
	fmt.Fprintln(w, "  or open certlm.msc (machine) / certmgr.msc (user).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  If your organization's root CA is missing, import it and retry:")
	fmt.Fprintln(w, "    Import-Certificate -FilePath <ca.crt> -CertStoreLocation Cert:\\CurrentUser\\Root")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  To trust a PEM bundle directly instead of the Windows store:")
	fmt.Fprintln(w, "    dr --ca-cert <path-to-ca.pem> <command>")
}
