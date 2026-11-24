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

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terminalPayload clears the screen, sets the window title through an OSC and
// rings the bell, between two words that must survive.
const terminalPayload = "keep\x1b[2J\x1b[31m\x1b]0;pwn\x07 this"

func TestClean_RemovesWhatReachesATerminalOrReadsAsAnInstruction(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"an escape sequence":       {"evil \x1b[2J\x1b[31mPWNED\x1b]0;pwn\x07 name", "evil [2J[31mPWNED]0;pwn name"},
		"a C1 control":             {"a\u009bmb", "amb"},
		"a bidi override":          {"gnp‮.exe", "gnp.exe"},
		"a zero width space":       {"a​b", "ab"},
		"a Unicode tag character":  {"ok\U000E0041\U000E0042", "ok"},
		"a private use character":  {"ab", "ab"},
		"a line separator":         {"a b", "a b"},
		"a paragraph separator":    {"a b", "a b"},
		"invalid UTF-8":            {"a\xffb\xc3", "ab"},
		"a newline":                {"line one\nline two", "line one line two"},
		"a run of blanks":          {"  a \t\r\n b  ", "a b"},
		"plain text":               {"[DataRobot] Python 3.12 Applications Base", "[DataRobot] Python 3.12 Applications Base"},
		"text in another language": {"環境 ünï", "環境 ünï"},
		"nothing":                  {"", ""},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, clean(c.in))
		})
	}
}

func TestClean_CutsALongStringAtTheLimit(t *testing.T) {
	got := clean(strings.Repeat("é", textLimit+50))

	assert.Equal(t, textLimit, utf8.RuneCountInString(got))
}

func TestClean_KeepsAStringAtTheLimit(t *testing.T) {
	in := strings.Repeat("a", textLimit)

	assert.Equal(t, in, clean(in))
}

// unsafeRune reports a rune that must not reach a report string.
func unsafeRune(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs, unicode.Zl, unicode.Zp) || r == utf8.RuneError
}

// assertAllStringsSafe fails for every string in a decoded JSON value that holds
// a rune clean removes.
func assertAllStringsSafe(t *testing.T, path string, v any) {
	t.Helper()

	switch value := v.(type) {
	case string:
		assert.False(t, strings.ContainsFunc(value, unsafeRune), "%s holds an unsafe character: %q", path, value)
	case []any:
		for _, item := range value {
			assertAllStringsSafe(t, path+"[]", item)
		}
	case map[string]any:
		for key, item := range value {
			assertAllStringsSafe(t, path+"."+key, key)
			assertAllStringsSafe(t, path+"."+key, item)
		}
	}
}

func hostile(name string) string {
	return name + "\u001b[2J\u001b]0;pwn\u0007‮​"
}

// Every route answers with hostile strings. No string of the report may keep a
// control, format, private use, separator or invalid character, and a name the
// caller did not ask for must not appear.
func TestDescribe_NoServerStringKeepsACharacterThatReachesATerminalOrAReader(t *testing.T) {
	fake := newFakeInstall(t, "sts-11.12.0")
	fake.set("GET /config", route{body: jsonOf(t, map[string]any{
		"RELEASE_VERSION":                    hostile("11.12.0"),
		"EXTERNAL_WEB_SERVER_URL":            hostile("https://dr.example.com"),
		"IS_ENTERPRISE":                      true,
		"CUSTOM_APP_DEFAULT_RESOURCE_BUNDLE": hostile("cpu.small"),
	})})
	fake.set("GET /api/v2/version/", route{body: jsonOf(t, map[string]any{"versionString": hostile("2.48")})})
	fake.set("GET /api/v2/account/info/", route{body: jsonOf(t, map[string]any{
		"seatLicenses": map[string]bool{hostile("SEAT"): true},
	})})
	fake.set("POST /api/v2/entitlements/evaluate/", route{body: jsonOf(t, map[string]any{
		"entitlements": []map[string]any{{"name": hostile("NOT_ASKED_FOR"), "value": true}},
	})})
	fake.set("GET /api/v2/executionEnvironments/", route{body: jsonOf(t, map[string]any{
		"data": []map[string]any{{
			"id": hostile("e1"), "name": hostile("env"), "programmingLanguage": hostile("python"),
			"useCases":                []string{hostile("customApplication")},
			"latestSuccessfulVersion": map[string]any{"sourceDockerImageUri": hostile("registry.example.com/base:1")},
			"latestVersion":           map[string]any{"buildStatus": hostile("success")},
		}},
	})})
	fake.set("GET /api/v2/mlops/compute/bundles/", route{body: jsonOf(t, map[string]any{
		"data": []map[string]any{{"id": hostile("b1"), "name": hostile("bundle"), "useCases": []string{hostile("customApplication")}}},
	})})
	fake.set("GET /api/v2/workloads/", route{status: http.StatusForbidden, body: hostile("no seat")})

	report, err := Describe(context.Background())
	require.NoError(t, err)

	var out bytes.Buffer

	require.NoError(t, RenderTo(&out, outputformat.OutputFormatJSON, *report))

	var decoded any

	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assertAllStringsSafe(t, "report", decoded)
	assert.NotContains(t, out.String(), "NOT_ASKED_FOR")
}
