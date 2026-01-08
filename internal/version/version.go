// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package version

import (
	"fmt"
	"runtime"
)

const CliName = "dr"

const AppName = "DataRobot CLI"

var Version = "dev"

// GitCommit is the commit hash of the current version.
var GitCommit = "unknown"

// BuildDate is the date when the binary was built.
var BuildDate = "unknown"

type InfoData struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Runtime   string `json:"runtime"`
}

var (
	FullVersion string
	Info        InfoData
)

func init() {
	Info = InfoData{
		Version:   Version,
		Commit:    GitCommit,
		BuildDate: BuildDate,
		Runtime:   runtime.Version(),
	}

	FullVersion = fmt.Sprintf("%s (commit: %s, built date: %s, runtime: %s)", Version, GitCommit, BuildDate, runtime.Version())
}

func GetAppNameFullVersionText() string {
	return AppName + " version: " + FullVersion
}

func GetAppNameVersionText() string {
	return AppName + " version: " + Version
}

func GetAppNameWithVersion() string {
	return fmt.Sprintf("%s (version %s)", AppName, Version)
}
