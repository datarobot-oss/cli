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

package telemetry

import (
	"os"
	"strings"
	"sync"
)


var (
	languageOnce  sync.Once
	languageCache string
)

// langFromEnv reads the language tag from LANG or LANGUAGE environment
// variables, stripping the encoding suffix (e.g. "en_US.UTF-8" → "en_US").
func langFromEnv() string {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LANGUAGE")
	}

	if lang == "" {
		return ""
	}

	if idx := strings.Index(lang, "."); idx != -1 {
		lang = lang[:idx]
	}

	return lang
}

// detectLanguage returns the user's BCP 47 language tag (e.g. "en_US").
// The result is computed once and cached for the lifetime of the process.
// Returns empty string if detection fails.
func detectLanguage() string {
	languageOnce.Do(func() {
		languageCache = osLanguage()
	})

	return languageCache
}
