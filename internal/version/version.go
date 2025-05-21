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

// GitCommit is the commit hash of the current version.
var GitCommit = "unknown"

// BuildDate is the date when the binary was built.
var BuildDate = "unknown"

var FullVersion string

func init() {
	FullVersion = fmt.Sprintf("%s (%s, runtime: %s)", GitCommit, BuildDate, runtime.Version())
}
