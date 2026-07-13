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

package image

import (
	"github.com/datarobot/cli/cmd/pipeline/image/create"
	"github.com/datarobot/cli/cmd/pipeline/image/del"
	"github.com/datarobot/cli/cmd/pipeline/image/get"
	"github.com/datarobot/cli/cmd/pipeline/image/list"
	"github.com/datarobot/cli/cmd/pipeline/image/update"
	"github.com/datarobot/cli/cmd/pipeline/image/version"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipeline image`. It groups the
// lifecycle verbs that operate on pipeline execution images (named,
// immutable-versioned execution environments).
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"images"},
		Short:   "Manage pipeline execution images",
		Long: `Manage pipeline execution images.

Images are named, immutable-versioned execution environments backed by
pip packages, conda packages, a base Docker image, and optional NVIDIA GPU
support. Pipelines can be built against them. Each ` + "`update`" + ` creates
a new version with a complete replacement definition; older versions can be
deleted individually with ` + "`image version delete`" + `.`,
	}

	cmd.AddCommand(
		create.Cmd(),
		get.Cmd(),
		list.Cmd(),
		update.Cmd(),
		del.Cmd(),
		version.Cmd(),
	)

	return cmd
}
