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

package wizard

import (
	"math"
	"math/bits"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// EnvKind is what a `.env` entry looks like, and therefore where it belongs.
type EnvKind int

const (
	// EnvConfig is an ordinary setting: it belongs in the manifest as a
	// literal, and there is nothing to hide about it.
	EnvConfig EnvKind = iota
	// EnvSecret must not be committed. It belongs in the credential store,
	// referenced from the manifest by id.
	EnvSecret
	// EnvLocal only means something on a developer's machine, so deploying it
	// would be wrong rather than merely unnecessary.
	EnvLocal
)

func (k EnvKind) String() string {
	switch k {
	case EnvSecret:
		return "secret"
	case EnvLocal:
		return "local only"
	case EnvConfig:
		return "config"
	}

	return "config"
}

// EnvVar is one classified `.env` entry.
//
// Value is kept so an ordinary setting can be written into the manifest as
// the literal it is, which is what makes the deploy carry the app's
// configuration. It is kept for a secret too, because the table lets a row be
// reclassified and a value dropped on the way through could not come back;
// what stops a secret reaching the file is the render, which writes a
// credential reference for one and never its value.
type EnvVar struct {
	Name  string
	Kind  EnvKind
	Value string
	// Reason is the short phrase the table shows, so the user can tell a
	// confident verdict from a guess before overriding it.
	Reason string
}

// Classification is deliberately conservative in one direction: it would
// rather call an ordinary setting a secret than the reverse, because the cost
// of the first is one keystroke on the table and the cost of the second is a
// credential in git. Every verdict is overridable, which is what makes an
// imperfect classifier safe.
//
// The signals below run in confidence order, strongest first.

// secretValuePatterns are issuer-specific token shapes. A value matching one
// of these is a secret whatever it is called, and these are the only signals
// strong enough to overrule a name that says otherwise.
var secretValuePatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`^-----BEGIN [A-Z ]*PRIVATE KEY-----`), "a private key"},
	{regexp.MustCompile(`^sk-ant-[A-Za-z0-9_-]{20,}`), "an Anthropic API key"},
	{regexp.MustCompile(`^sk-[A-Za-z0-9_-]{20,}`), "an OpenAI-style API key"},
	{regexp.MustCompile(`^(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{30,}`), "a GitHub token"},
	{regexp.MustCompile(`^github_pat_[A-Za-z0-9_]{20,}`), "a GitHub token"},
	{regexp.MustCompile(`^glpat-[A-Za-z0-9_-]{15,}`), "a GitLab token"},
	{regexp.MustCompile(`^xox[baprs]-[A-Za-z0-9-]{10,}`), "a Slack token"},
	{regexp.MustCompile(`^hf_[A-Za-z0-9]{20,}`), "a Hugging Face token"},
	{regexp.MustCompile(`^AKIA[0-9A-Z]{16}$`), "an AWS access key id"},
	{regexp.MustCompile(`^AIza[0-9A-Za-z_-]{35}$`), "a Google API key"},
	{regexp.MustCompile(`^SG\.[A-Za-z0-9_-]{20,}`), "a SendGrid key"},
	{regexp.MustCompile(`^dop_v1_[a-f0-9]{64}$`), "a DigitalOcean token"},
	{regexp.MustCompile(`^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.`), "a JSON web token"},
}

// secretNamePattern is the design's name test. On its own it is a hint, not a
// verdict: SSH_KEY_PATH and LOG_LEVEL_KEY both match and neither is a secret,
// which is why the value has to corroborate.
var secretNamePattern = regexp.MustCompile(
	`(?i)(^|[_.-])(KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD|CREDENTIALS?|AUTH|APIKEY|PRIVATE|SALT|SIGNATURE|CERT)($|[_.-])`)

// pathishNamePattern marks names whose value is a location rather than the
// thing at that location. SSH_KEY_PATH names a file; the file is the secret,
// the path is not.
var pathishNamePattern = regexp.MustCompile(`(?i)(^|_)(PATH|FILE|DIR|LOCATION|URL|URI|ENDPOINT|HOST)$`)

// localNamePattern marks variables that only mean something while developing.
// The frontend prefixes are build-time and public by construction, so a
// secret in one is already a different problem.
var localNamePattern = regexp.MustCompile(`(?i)^(VITE_|REACT_APP_|NEXT_PUBLIC_|STORYBOOK_|EXPO_PUBLIC_)`)

// devNamePattern marks the dev-only half of a name that is otherwise ordinary.
var devNamePattern = regexp.MustCompile(`(?i)(^|_)(DEV|LOCAL|DEBUG|MOCK|FIXTURE|SANDBOX)($|_)`)

// localHostPattern matches values that point at the developer's own machine.
var localHostPattern = regexp.MustCompile(`(?i)(^|//|@|\b)(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]|host\.docker\.internal)(\b|:|/|$)`)

// plainValuePattern matches values that cannot be a secret whatever they are
// called: booleans, numbers, and short bare words such as log levels.
var plainValuePattern = regexp.MustCompile(`^(?i)(true|false|yes|no|on|off|null|none|debug|info|warn|warning|error|trace|fatal|silent|dev|development|prod|production|staging|test)$`)

// minSecretLength is the shortest value entropy is allowed to convict. Below
// it, a high score says more about the sample size than the value.
const minSecretLength = 16

// minSecretEntropy is Shannon entropy per character. English prose sits near
// 2.5 and a random token near 4.5, so this sits between them and closer to
// the token, because a false secret costs a keystroke and a false config
// costs a leak.
const minSecretEntropy = 3.4

// ClassifyEnv decides where one `.env` entry belongs. name and value are both
// read; only the verdict is returned.
func ClassifyEnv(name, value string) EnvVar {
	value = strings.TrimSpace(value)

	for _, signal := range secretValuePatterns {
		if signal.pattern.MatchString(value) {
			return EnvVar{Name: name, Kind: EnvSecret, Reason: "looks like " + signal.reason}
		}
	}

	if user, ok := credentialInURL(value); ok {
		return EnvVar{Name: name, Kind: EnvSecret, Reason: "a URL with " + user + "'s password in it"}
	}

	if kind, reason, ok := classifyLocal(name, value); ok {
		return EnvVar{Name: name, Kind: kind, Value: value, Reason: reason}
	}

	return classifyBySecrecy(name, value)
}

// classifyLocal catches what only matters on this machine. It runs before the
// name test so a VITE_ variable is not called a secret for containing KEY,
// and before entropy so a localhost URL is not convicted for looking random.
func classifyLocal(name, value string) (EnvKind, string, bool) {
	switch {
	case localNamePattern.MatchString(name):
		return EnvLocal, "a build-time frontend variable", true
	case localHostPattern.MatchString(value):
		return EnvLocal, "points at this machine", true
	case devNamePattern.MatchString(name) && !secretNamePattern.MatchString(name):
		return EnvLocal, "named for local development", true
	default:
		return EnvConfig, "", false
	}
}

// classifyBySecrecy weighs the name against the value, which is the case the
// design's name-pattern test gets wrong on its own.
func classifyBySecrecy(name, value string) EnvVar {
	if secretNamePattern.MatchString(name) && !pathishNamePattern.MatchString(name) {
		return classifyNamedSecret(name, value)
	}

	if looksGenerated(value) {
		// A DATABASE_URL-style name with an opaque token inside it.
		return EnvVar{Name: name, Kind: EnvSecret, Reason: "the value looks like a random token"}
	}

	return EnvVar{Name: name, Kind: EnvConfig, Value: value, Reason: "an ordinary setting"}
}

// classifyNamedSecret settles a name that matched the secret pattern, which
// is a hint the value can confirm or contradict.
func classifyNamedSecret(name, value string) EnvVar {
	switch {
	case plainValuePattern.MatchString(value):
		// LOG_LEVEL_KEY=debug. The name matched; the value cannot be a secret.
		return EnvVar{Name: name, Kind: EnvConfig, Value: value, Reason: "named like a secret, but the value is a plain setting"}
	case value == "":
		return EnvVar{Name: name, Kind: EnvSecret, Reason: "named like a secret, and empty here"}
	case looksGenerated(value):
		return EnvVar{Name: name, Kind: EnvSecret, Reason: "named like a secret, and the value looks random"}
	default:
		return EnvVar{Name: name, Kind: EnvSecret, Reason: "named like a secret"}
	}
}

// looksGenerated reports whether a value looks machine-generated rather than
// chosen. A URL is judged by its parts rather than as a whole: scheme, host
// and ordinary path segments are structure and score high on character
// distribution without being secret, but an opaque segment inside the path is
// exactly how webhook URLs carry theirs.
func looksGenerated(value string) bool {
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return anySegmentGenerated(parsed)
	}

	return isRandomToken(value)
}

// anySegmentGenerated looks for an opaque token inside a URL's path or query.
func anySegmentGenerated(parsed *url.URL) bool {
	for _, segment := range strings.Split(parsed.Path, "/") {
		if isRandomToken(segment) {
			return true
		}
	}

	for _, values := range parsed.Query() {
		for _, value := range values {
			if isRandomToken(value) {
				return true
			}
		}
	}

	return false
}

// isRandomToken is the entropy test proper: long enough to measure, surprising
// enough per character, and drawing on more than one class of character.
func isRandomToken(value string) bool {
	return len(value) >= minSecretLength &&
		shannonEntropy(value) >= minSecretEntropy &&
		mixedCharset(value)
}

// credentialInURL reports a password embedded in a connection string, which
// is a secret however innocuous DATABASE_URL sounds.
func credentialInURL(value string) (string, bool) {
	if !strings.Contains(value, "://") {
		return "", false
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.User == nil {
		return "", false
	}

	password, set := parsed.User.Password()
	if !set || password == "" {
		return "", false
	}

	user := parsed.User.Username()
	if user == "" {
		user = "a user"
	}

	return user, true
}

// shannonEntropy is bits per character over the value's own distribution: how
// surprising the next character is, which is what separates a generated token
// from a word someone chose.
func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}

	counts := make(map[rune]int, len(value))
	for _, r := range value {
		counts[r]++
	}

	total := float64(len([]rune(value)))

	var entropy float64

	for _, count := range counts {
		p := float64(count) / total
		entropy -= p * math.Log2(p)
	}

	return entropy
}

// mixedCharset guards entropy against long structured values. A file path or
// a sentence can score well on character distribution alone, so a value only
// counts as random if it also draws on more than one class of character and
// is not something as ordinary as a number.
func mixedCharset(value string) bool {
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return false
	}

	const (
		lower = 1 << iota
		upper
		digit
	)

	var seen int

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			seen |= lower
		case r >= 'A' && r <= 'Z':
			seen |= upper
		case r >= '0' && r <= '9':
			seen |= digit
		}
	}

	// Two classes is the bar: hex digests and mixed-case tokens both clear
	// it, while /usr/local/share/some-long-path and a plain sentence do not.
	return bits.OnesCount(uint(seen)) >= 2
}
