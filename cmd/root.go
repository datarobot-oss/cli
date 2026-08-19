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

// Package cmd is the top-level cobra command package for the DataRobot CLI.
//
// Production entry-point: main.go calls ExecuteContext, which delegates to the
// package-level productionFactory singleton built by init(). Sub-commands and
// tests can call NewRootFactory (with functional options to override
// dependencies) to obtain a fully-isolated command tree without touching any
// package-level state.
package cmd

import (
	"context"
	"fmt"

	"github.com/datarobot/cli/cmd/allcommands"
	selfversion "github.com/datarobot/cli/cmd/self/version"
	"github.com/datarobot/cli/internal/cli"
	"github.com/spf13/cobra"
)

// productionFactory is the singleton RootFactory used by the running binary.
// It is initialised in init() so that tests importing this package still get
// a fully-wired command tree through the RootCmd convenience variable.
var productionFactory *RootFactory

// RootCmd is the built root command from the production factory. It exists as
// a package-level variable so existing tests and callers that reference
// cmd.RootCmd continue to work without modification.
//
// For test isolation, prefer creating a dedicated factory with NewRootFactory
// and calling Build() on it rather than sharing this singleton.
var RootCmd *cli.CommandAdder

// init wires up the package-level variables and registers the allcommands /
// version helper functions that the factory's help override needs.
//
// These helpers cannot live inside root_factory.go itself because they import
// packages that would create an import cycle (allcommands imports cmd
// sub-packages; selfversion imports internal packages that indirectly
// reference cmd). Registering function values here breaks the cycle while
// keeping the factory import-light.
func init() {
	allCommandsOutputFn = func(root *cobra.Command) string {
		return allcommands.GenerateCommandTree(root)
	}

	runVersionCommandFn = func(cmd *cobra.Command) {
		versionCmd := selfversion.Cmd()

		// Suppress cobra's automatic error/usage output to prevent double-printing.
		versionCmd.SilenceErrors = true
		versionCmd.SilenceUsage = true
		versionCmd.SetArgs([]string{"--output-format", "text"})
		versionCmd.SetOut(cmd.OutOrStdout())
		versionCmd.SetErr(cmd.ErrOrStderr())

		if err := versionCmd.Execute(); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		} else {
			// Only print the update hint on success.
			fmt.Fprintln(cmd.OutOrStdout(), "\nTo update: dr self update")
		}
	}

	productionFactory = NewRootFactory()
	RootCmd = productionFactory.Build()
}

// ExecuteContext executes the root command with the given context.
// This is called by main.main(). It only needs to happen once per process.
func ExecuteContext(ctx context.Context) error {
	if err := RootCmd.ExecuteContext(ctx); err != nil {
		return fmt.Errorf("execute root command: %w", err)
	}

	return nil
}
