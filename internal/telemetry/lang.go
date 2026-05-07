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
	"sync"

	golocale "github.com/jeandeaual/go-locale"
)

var (
	languageOnce  sync.Once
	languageCache string
)

// detectLanguage returns the user's BCP 47 language tag (e.g. "en_US").
// Amplitude will automatically map this tag to a more general language name.
// The result is computed once and cached for the lifetime of the process.
// Returns empty string if detection fails.
// Ref: https://amplitude.com/docs/apis/analytics/http-v2#language-field
func detectLanguage() string {
	languageOnce.Do(func() {
		lang, err := golocale.GetLocale()
		if err == nil {
			languageCache = lang
		}
	})

	return languageCache
}
