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

package log

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

const logLevelWidth = 5

// logFile is the filename for logs
const logFile = "dr-tui-debug.log"

// logStyles customizes the log styles for logging
var logStyles *log.Styles

func init() {
	logStyles = log.DefaultStyles()
	for l, style := range logStyles.Levels {
		logStyles.Levels[l] = style.MaxWidth(logLevelWidth).PaddingRight(1)
	}
}

var (
	level        log.Level
	fileWriter   io.WriteCloser
	stderrLogger *log.Logger
	fileLogger   *log.Logger
)

func Start(useDebug, useVerbose bool) {
	level = log.Default().GetLevel()

	// Debug takes precedence
	if useDebug {
		level = log.DebugLevel
	} else if useVerbose {
		level = log.InfoLevel
	}

	StartStderr()
	StartFile()
}

func Stop() {
	StopFile()
	StopStderr()
}

func StartStderr() {
	stderrLogger = log.New(os.Stderr)
	stderrLogger.SetStyles(logStyles)
	stderrLogger.SetLevel(level)
}

func StopStderr() {
	stderrLogger = nil
}

func StartFile() {
	var err error

	fileWriter, err = os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}

	fileLogger = log.New(fileWriter)
	fileLogger.SetStyles(logStyles)
	fileLogger.SetLevel(level)
}

func StopFile() {
	fileLogger = nil

	fileWriter.Close()
}

func SetLogLevelFromConfig() {
	if viper.GetBool("debug") {
		level = log.DebugLevel
	} else if viper.GetBool("verbose") {
		level = log.InfoLevel
	} else {
		level = log.Default().GetLevel()
	}
}

func Log(level log.Level, msg interface{}, keyvals ...interface{}) {
	if stderrLogger != nil {
		stderrLogger.Log(level, msg, keyvals...)
	}

	if fileLogger != nil {
		fileLogger.Log(level, msg, keyvals...)
	}
}

func Logf(level log.Level, format string, args ...interface{}) {
	if stderrLogger != nil {
		stderrLogger.Logf(level, format, args...)
	}

	if fileLogger != nil {
		fileLogger.Logf(level, format, args...)
	}
}
