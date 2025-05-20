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
