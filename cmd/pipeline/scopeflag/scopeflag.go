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

// Package scopeflag bundles the --pipeline / --scope / --version flags
// reused by the input and run CLI command groups.

package scopeflag

import (
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

// Flags holds the values backing the shared --pipeline / --scope /
// --version flags. Bind() registers them on a cobra command and
// Resolve(cmd) turns them into a (Scope, *version) pair via
// pipeline.ResolveScope.
type Flags struct {
	PipelineID string
	Scope      string
	Version    int
}

// Bind registers --pipeline, --scope and --version on cmd. The caller is
// responsible for marking --pipeline required if appropriate.
func (f *Flags) Bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.PipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().StringVar(&f.Scope, "scope", "", "Scope: draft (default) or locked (auto-set when --version is supplied)")
	cmd.Flags().IntVar(&f.Version, "version", 0, "Pipeline version (implies --scope=locked)")
}

// Resolve combines the parsed flags into the canonical (Scope, *version)
// pair required by the pipelines client. It must be called from RunE (or
// later) so cmd.Flags().Changed has accurate state.
func (f *Flags) Resolve(cmd *cobra.Command) (pipeline.Scope, *int, error) {
	var version *int

	if cmd.Flags().Changed("version") {
		v := f.Version

		version = &v
	}

	return pipeline.ResolveScope(f.Scope, version)
}
