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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/joho/godotenv"
)

// DockerfileName is the only Dockerfile setup looks for, at the root of the
// project directory. A build elsewhere is a hand edit, not an answer the
// wizard offers.
const DockerfileName = "Dockerfile"

// DefaultDockerfilePath is how the --dockerfile flag spells that same file.
// The flag takes no other value: the platform builds the project root's
// Dockerfile, and a flag that quietly did nothing would be worse than one
// that says so.
const DefaultDockerfilePath = "./" + DockerfileName

// EnvFileName is the local environment file the wizard looks for. It is read
// in full: `up` deploys the manifest and never reads .env, so anything kept
// only there would not reach the container. Ordinary settings are copied into
// the manifest as literals and secrets become credential references, which is
// why the values are classified rather than merely counted.
const EnvFileName = ".env"

// maxDockerfileSize bounds the read. A Dockerfile is a few kilobytes; a
// hundred is either not a Dockerfile or not one worth scanning for EXPOSE.
const maxDockerfileSize = 1 << 20

// maxEnvFileSize bounds the same way. A .env is a handful of lines; anything
// larger is not one, and the count is only used for a one-line notice.
const maxEnvFileSize = 1 << 20

// Detected is what the project says about itself before anything is asked.
// Every field here is a default the user can override on the screen it
// belongs to.
type Detected struct {
	// Dir is the absolute project directory the manifest will be written to.
	Dir string
	// Name is the suggested workload name, taken from the directory.
	Name string
	// HasDockerfile decides which image source leads on the Q4 screen.
	HasDockerfile bool
	// Port is the suggested container port.
	Port int
	// PortSource explains where Port came from, shown next to the answer so
	// the user can tell a detected value from a guess.
	PortSource string
	// EnvVars are the variables the project's .env defines, classified, in
	// the order the file defines them; empty when there is no .env.
	//
	// `up` deploys the manifest and never reads .env, so a project that keeps
	// its configuration there would otherwise deploy without any of it and
	// find out at runtime. The classification decides how each one is
	// written: an ordinary value as a literal, a secret as a credential
	// reference whose id the user fills in. Every row is overridable.
	EnvVars []EnvVar
	// EnvErr is why a .env that is present contributed nothing. Setup still
	// proceeds, because a manifest without the import is a working manifest,
	// but it must be reported: silence here reads as "there was nothing to
	// import" and the missing variables surface as a runtime failure instead.
	EnvErr error
}

// HasEnvFile reports whether the project has a .env worth asking about.
func (d Detected) HasEnvFile() bool {
	return len(d.EnvVars) > 0
}

// Plural picks the singular or plural word for count. Exported so the
// command's summary reads the same way the screens do.
func Plural(count int, one, many string) string {
	if count == 1 {
		return one
	}

	return many
}

// Detect reads the project directory. It never fails: anything it cannot
// determine falls back to a documented default, and the screens make every
// value visible before it is written.
func Detect(dir string) Detected {
	detected := Detected{
		Dir:           dir,
		Name:          sanitizeName(filepath.Base(dir)),
		HasDockerfile: fsutil.FileExists(filepath.Join(dir, DockerfileName)),
		Port:          manifest.DefaultPort,
		PortSource:    "the default for a web service; edit if yours differs",
	}

	detected.EnvVars, detected.EnvErr = envVars(filepath.Join(dir, EnvFileName))

	if !detected.HasDockerfile {
		return detected
	}

	port, ok := exposedPort(filepath.Join(dir, DockerfileName))
	if !ok {
		return detected
	}

	// A privileged port cannot be honored: containers run unprivileged, so a
	// primary container below 1024 never binds and the workload never becomes
	// ready. Suggesting it would only produce a file the ledger rejects, so
	// the default stands and the screen says why.
	if port < manifest.MinPrimaryPort {
		detected.PortSource = fmt.Sprintf(
			"your %s says EXPOSE %d, but containers run unprivileged and cannot bind below %d",
			DockerfileName, port, manifest.MinPrimaryPort)

		return detected
	}

	detected.Port = port
	detected.PortSource = fmt.Sprintf("found EXPOSE %d in your %s", port, DockerfileName)

	return detected
}

// exposedPort returns the first port an EXPOSE line names. Later EXPOSE lines
// are ignored: a multi-port image still has one port traffic arrives on, and
// guessing among them would be worse than showing the first and letting the
// user correct it.
func exposedPort(path string) (int, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxDockerfileSize {
		return 0, false
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.EqualFold(fields[0], "EXPOSE") {
			continue
		}

		// EXPOSE 8080/tcp is legal, and so is EXPOSE $PORT, which tells us
		// nothing because the value only exists at build time.
		port, err := strconv.Atoi(strings.SplitN(fields[1], "/", 2)[0])
		if err != nil {
			continue
		}

		return port, true
	}

	return 0, false
}

// envVars reads and classifies a .env, and reports separately why a file that
// exists yielded nothing. Both returns are empty when there is no .env at all,
// which is not a problem and has nothing to say. The values travel with the
// verdicts: an ordinary setting is written to the manifest as it stands, and a
// secret's value is what a later credential is created from.
//
// One read serves both purposes: godotenv returns a map, so the same bytes are
// walked again for the order it loses, a scrambled list being one the user
// could not match against their editor.
func envVars(path string) ([]EnvVar, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}

	if info.Size() > maxEnvFileSize {
		return nil, fmt.Errorf("%s is %d bytes, larger than the %d this reads", path, info.Size(), maxEnvFileSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	text := string(content)

	// Parsed from the bytes already in hand rather than from the path again,
	// so the keys and the order they are recovered in come from one read.
	values, err := godotenv.Unmarshal(text)
	if err != nil {
		// A .env this parser cannot read is still a .env, but listing keys we
		// are not sure of would be worse than listing none. The parser's own
		// error is deliberately not wrapped; see parseProblem.
		return nil, fmt.Errorf("cannot parse %s: %s", path, parseProblem(err, text))
	}

	classified := make([]EnvVar, 0, len(values))
	for _, name := range orderedKeys(text, values) {
		classified = append(classified, ClassifyEnv(name, values[name]))
	}

	return classified, nil
}

// parseProblem says why a .env would not parse without repeating any of it.
// The parser reports the unparsed remainder of the input as part of its
// message, so a malformed name on the second line would print every value
// below it, and a headless run prints that to stderr and from there into
// whatever log is capturing the build. Only the fixed half of a message this
// code recognizes is repeated; anything else is described generically,
// because a message it has not seen cannot be assumed free of file content.
func parseProblem(err error, content string) string {
	msg := err.Error()

	// Tested before the split below, and by its opening rather than by any
	// substring: this message ends in the raw value, so a value that happened
	// to contain " near " would otherwise be cut in half and the first half
	// printed.
	if strings.HasPrefix(msg, unterminatedQuotedValue) {
		return "a quoted value is never closed"
	}

	// "unexpected character %q in variable name near %q". What precedes
	// " near " holds only the offending character, which comes from a name
	// and is the one detail worth repeating; the rest is the payload.
	if strings.HasPrefix(msg, unexpectedCharacter) {
		if fixed, remainder, found := strings.Cut(msg, " near "); found {
			return fixed + atLine(content, remainder)
		}
	}

	return "it is not in KEY=value form"
}

// The godotenv messages this code repeats any part of. Each continues with
// text lifted out of the file, so each is recognized by its fixed opening;
// a message that matches neither is described in this package's own words.
const (
	unterminatedQuotedValue = "unterminated quoted value"
	unexpectedCharacter     = "unexpected character "
)

// atLine converts the remainder the parser choked on into the line number it
// begins at, which is the one part of that payload worth showing. The
// remainder is a suffix of the file, so everything before it is what parsed.
// An unrecognizable payload yields no line rather than a guessed one.
func atLine(content, quotedRemainder string) string {
	remainder, err := strconv.Unquote(quotedRemainder)
	if err != nil || !strings.HasSuffix(content, remainder) {
		return ""
	}

	return fmt.Sprintf(" on line %d", strings.Count(content[:len(content)-len(remainder)], "\n")+1)
}

// orderedKeys walks the file to recover the order godotenv's map loses, so
// the listing matches what the user sees in their editor. Only names the
// parser agreed on are kept, which is what filters comments and blank lines.
func orderedKeys(content string, values map[string]string) []string {
	keys := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))

	for _, line := range strings.Split(content, "\n") {
		name, ok := assignedName(line)
		if !ok || seen[name] {
			continue
		}

		if _, declared := values[name]; !declared {
			continue
		}

		seen[name] = true

		keys = append(keys, name)
	}

	return keys
}

// assignedName returns the variable a line assigns, if it assigns one.
func assignedName(line string) (string, bool) {
	name, _, found := strings.Cut(strings.TrimSpace(line), "=")
	if !found {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(name, "export ")), true
}

// sanitizeName turns a directory name into a plausible workload name. The
// platform's own rules are enforced server-side; this only removes what a
// directory can carry and a name should not, so the suggestion is usable
// without editing.
func sanitizeName(dir string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r == ' ', r == '.':
			return '-'
		default:
			return -1
		}
	}, dir)

	name = strings.Trim(name, "-_")
	if name == "" {
		return "my-app"
	}

	return name
}
