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

// Package viperx is the curated entry point to viper for the rest of the
// CLI. It re-exports only the safe subset of viper's API that the project
// is permitted to use.
//
// Why this package exists:
//
//   - viper.WriteConfig and viper.SafeWriteConfig serialize the entire
//     viper.AllSettings() map, which includes every flag that has ever
//     been bound (e.g. --yes, --verbose). Calling them directly leaks
//     transient flag state into drconfig.yaml. Production code must
//     instead call config.UpdateConfigFile, which writes only the
//     allowlisted keys in config.PersistableKeys.
//
//   - viper.BindPFlags(cmd.Flags()) bulk-binds every subcommand flag to
//     viper, with the same leakage problem. Persistent root flags are
//     bound individually via BindPFlag in cmd/root.go::init(); subcommand
//     flags are read directly from cobra (cmd.Flags().GetX(...)).
//
// Both APIs are intentionally NOT re-exported here. Direct imports of
// github.com/spf13/viper outside internal/config/** are forbidden by
// depguard. See docs/development/configuration.md for the full contract.
//
// To add a symbol:
//
//  1. Confirm it is safe (does not persist transient state).
//  2. Add a re-export below.
//  3. Document in docs/development/configuration.md if the addition
//     introduces a new pattern.
package viperx

import (
	"github.com/spf13/viper"
)

// ConfigFileNotFoundError mirrors viper.ConfigFileNotFoundError so
// callers can do errors.As against it without importing viper.
type ConfigFileNotFoundError = viper.ConfigFileNotFoundError

// Reads / state inspection.
var (
	Get            = viper.Get
	GetString      = viper.GetString
	GetBool        = viper.GetBool
	GetInt         = viper.GetInt
	GetDuration    = viper.GetDuration
	IsSet          = viper.IsSet
	AllSettings    = viper.AllSettings
	ConfigFileUsed = viper.ConfigFileUsed
)

// Writes to live (in-memory) state. These do NOT persist to disk; use
// config.UpdateConfigFile for that.
var (
	Set        = viper.Set
	SetDefault = viper.SetDefault
	Reset      = viper.Reset
)

// Bindings. BindPFlags (the bulk binder) is intentionally omitted.
var (
	BindEnv           = viper.BindEnv
	BindPFlag         = viper.BindPFlag
	SetEnvPrefix      = viper.SetEnvPrefix
	SetEnvKeyReplacer = viper.SetEnvKeyReplacer
	AutomaticEnv      = viper.AutomaticEnv
)

// Read path: configuring how viper finds and parses the config file.
var (
	SetConfigType = viper.SetConfigType
	SetConfigName = viper.SetConfigName
	SetConfigFile = viper.SetConfigFile
	AddConfigPath = viper.AddConfigPath
	ReadInConfig  = viper.ReadInConfig
)
