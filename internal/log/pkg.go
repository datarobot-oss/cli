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
	"math"
	"os"

	"github.com/charmbracelet/log"
)

const (
	DebugLevel = log.DebugLevel
	InfoLevel  = log.InfoLevel
	WarnLevel  = log.WarnLevel
	ErrorLevel = log.ErrorLevel
	FatalLevel = log.FatalLevel
	noLevel    = log.Level(math.MaxInt)
)

func GetLevel() log.Level {
	return log.GetLevel()
}

func Debug(msg interface{}, keyvals ...interface{}) {
	Log(DebugLevel, msg, keyvals...)
}

func Info(msg interface{}, keyvals ...interface{}) {
	Log(InfoLevel, msg, keyvals...)
}

func Warn(msg interface{}, keyvals ...interface{}) {
	Log(WarnLevel, msg, keyvals...)
}

func Error(msg interface{}, keyvals ...interface{}) {
	Log(ErrorLevel, msg, keyvals...)
}

func Fatal(msg interface{}, keyvals ...interface{}) {
	Log(FatalLevel, msg, keyvals...)
	os.Exit(1)
}

func Print(msg interface{}, keyvals ...interface{}) {
	Log(noLevel, msg, keyvals...)
}

func Debugf(format string, args ...interface{}) {
	Log(DebugLevel, fmt.Sprintf(format, args...))
}

func Infof(format string, args ...interface{}) {
	Log(InfoLevel, fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...interface{}) {
	Log(WarnLevel, fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...interface{}) {
	Log(ErrorLevel, fmt.Sprintf(format, args...))
}

func Fatalf(format string, args ...interface{}) {
	Log(FatalLevel, fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Printf(format string, args ...interface{}) {
	Log(noLevel, fmt.Sprintf(format, args...))
}
