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

package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// validManifest exercises every ledger rule on its passing side: inline
// artifact with a flagged primary, both credential value forms, and a
// runtime block whose names match the artifact's.
const validManifest = `workloadId: 68b0aaaa0000000000000001
name: my-app
importance: HIGH
artifact:
  name: my-app-artifact
  status: draft
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
            environmentVars:
              - name: LOG_LEVEL
                value: debug
              - name: OPENAI_API_KEY
                value: dr-credential:68f0cccc0000000000000003/apiToken
runtime:
  containerGroups:
    - name: default
      replicaCount: 2
      containers:
        - name: primary
          resourceAllocation:
            cpu: 0.5
            memory: 512MB
`

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, FileName)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
