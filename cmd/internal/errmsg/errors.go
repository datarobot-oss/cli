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

// Package errmsg provides standardized error message templates for command handlers.
package errmsg

const (
	ResolveScope           = "resolve scope: %w"
	MarshalJSON            = "marshal json: %w"
	GetUserHomeDir         = "get user home dir: %w"
	CreateCompletionFile   = "create completion file: %w"
	CreateCompletionDir    = "create completion directory: %w"
	WriteZshrcFpath        = "write zshrc fpath: %w"
	WriteTarHeader         = "write tar header: %w"
	WalkSourceDirectory    = "walk source directory: %w"
	WalkSourceDir          = "walk source dir: %w"
	ValidatePluginScript   = "validate plugin script: %w"
	ResolveRelativePath    = "resolve relative path: %w"
	OpenFile               = "open file: %w"
	CreateArchiveFile      = "create archive file: %w"
	CopyFileIntoArchive    = "copy file into archive: %w"
	BuildTarHeader         = "build tar header: %w"
	OpenArchiveForChecksum = "open archive for checksum: %w"
	HashArchive            = "hash archive: %w"
	ResolvePayload         = "resolve payload: %w"
	ResolvePath            = "resolve file path: %w"
	ConfirmInstall         = "confirm install: %w"
	InstallPrerequisites   = "install prerequisites: %w"
	CreatePipeline         = "create pipeline: %w"
	ListPipelines          = "list pipelines: %w"
	LockPipeline           = "lock pipeline: %w"
	CreateImage            = "create image: %w"
	ListImages             = "list images: %w"
	UpdateImage            = "update image: %w"
	GetImageBuildLogs      = "get image build logs: %w"
	CreateInput            = "create input: %w"
	ListInputs             = "list inputs: %w"
	UpdateInput            = "update input: %w"
	GetInput               = "get input: %w"
	DeleteInput            = "delete input: %w"
	CreateRun              = "create run: %w"
	ListRuns               = "list runs: %w"
	GetRun                 = "get run: %w"
	CancelRun              = "cancel run: %w"
	GetRunStatus           = "get run status: %w"
	ListTaskExecutions     = "list task executions: %w"
	GetTaskLogs            = "get task logs: %w"
	GetTaskDurableLog      = "get task durable log: %w"
	GetTaskResult          = "get task result: %w"
	PrintGraphJSON         = "print graph json: %w"
	GetGraph               = "get graph: %w"
)
