// Copyright {{current_year}} DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package version

import (
	"fmt"
	"runtime"
)

const AppName = "dr"

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
