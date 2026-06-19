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

package workload

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Status string

var _ pflag.Value = (*Status)(nil)

func (s *Status) String() string {
	if s == nil {
		return ""
	}

	return string(*s)
}

func (s *Status) Set(v string) error {
	parsed, err := ParseArtifactStatus(v)
	if err != nil {
		return err
	}

	*s = Status(parsed)

	return nil
}

func (s *Status) Type() string {
	return "status"
}

func AddStatusFlag(cmd *cobra.Command, dest *Status) {
	cmd.Flags().Var(dest, "status", fmt.Sprintf("Filter by status (%s, %s)", ArtifactStatusDraft, ArtifactStatusLocked))
}
