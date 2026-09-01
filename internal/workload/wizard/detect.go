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
	"github.com/datarobot/cli/internal/log"
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
// in full: a deploy carries the manifest and never sends anything from .env,
// so anything kept only there would not reach the container. Ordinary settings are copied into
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
	// A deploy carries the manifest and sends nothing from .env, so a project
	// that keeps its configuration there would otherwise deploy without any of
	// it and find out at runtime. The classification decides how each one is
	// written: an ordinary value as a literal, a secret as a credential
	// reference whose id the user fills in. Every row is overridable.
	EnvVars []EnvVar
	// EnvErr is why a .env that is present contributed nothing. Setup still
	// proceeds, because a manifest without the import is a working manifest,
	// but it must be reported: silence here reads as "there was nothing to
	// import" and the missing variables surface as a runtime failure instead.
	EnvErr error
	// RootMarkers is which of the usual project files the directory itself
	// carries. See SuspectDir for what an empty list means.
	RootMarkers []string
	// Candidates are immediate subdirectories that do carry project files,
	// looked for only when the directory itself carries none: the offer the
	// wrong-directory question makes instead of assuming ".".
	Candidates []DirCandidate
	// MoreCandidates reports that the candidate scan stopped early — at the
	// offer cap or the entry budget — so the offer must say the list is not
	// the whole answer: a project whose name sorts late would otherwise
	// vanish without a trace, --dir unmentioned.
	MoreCandidates bool
}

// DirCandidate is a subdirectory that looks like a project root when the
// directory setup ran from does not.
type DirCandidate struct {
	// Rel is the candidate relative to the checked directory.
	Rel string
	// Markers is which project files it carries, so the offer says why it is
	// being offered rather than presenting a bare path.
	Markers []string
}

// rootMarkers are the files a project root usually carries. The list is for
// suspicion, not proof, in both directions: a directory with none can still
// be a deployable project, which is why nothing here refuses.
var rootMarkers = []string{
	DockerfileName, "pyproject.toml", "uv.lock", "requirements.txt", "package.json", "go.mod", "setup.py",
}

// maxDirCandidates caps the offer. Past a handful the list stops being an
// answer and --dir is the tool.
const maxDirCandidates = 6

// maxDirEntriesScanned caps the work, not just the offer: every entry
// examined costs a manifest stat plus one per root marker, synchronously
// before the wizard's first screen, and a directory with thousands of
// entries (running setup from $HOME by mistake being the classic) must not
// hang the wizard on stats.
const maxDirEntriesScanned = 256

// markersIn is which of the usual project files dir carries.
func markersIn(dir string) []string {
	var found []string

	for _, marker := range rootMarkers {
		if fsutil.FileExists(filepath.Join(dir, marker)) {
			found = append(found, marker)
		}
	}

	return found
}

// dirCandidates lists the immediate subdirectories carrying project files.
// Hidden directories are never the project, and one already holding a
// manifest is skipped too: it is entered with --dir, not set up afresh.
// truncated reports that the scan stopped before the listing did, at either
// cap, so a caller can say the offer is incomplete.
//
// The home directory is never scanned, same as repo.FindRepoRoot and
// manifest.Locate stop there: everything under $HOME "carrying project
// files" is not an offer, it is an inventory.
func dirCandidates(dir string) (candidates []DirCandidate, truncated bool) {
	if home, err := os.UserHomeDir(); err == nil && dir == home {
		log.Debug("wizard: not scanning the home directory for project candidates")

		return nil, false
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Debug("wizard: cannot list directory for project candidates", "dir", dir, "err", err)

		return nil, false
	}

	for i, entry := range entries {
		if i == maxDirEntriesScanned {
			return candidates, true
		}

		candidate, ok := candidateFrom(dir, entry)
		if !ok {
			continue
		}

		if len(candidates) == maxDirCandidates {
			return candidates, true
		}

		candidates = append(candidates, candidate)
	}

	return candidates, false
}

// candidateFrom examines one directory entry and reports the candidate it
// yields, if any.
func candidateFrom(dir string, entry os.DirEntry) (DirCandidate, bool) {
	if !isSubdirectory(dir, entry) || strings.HasPrefix(entry.Name(), ".") {
		return DirCandidate{}, false
	}

	sub := filepath.Join(dir, entry.Name())
	if fsutil.FileExists(manifest.Path(sub)) {
		return DirCandidate{}, false
	}

	markers := markersIn(sub)
	if len(markers) == 0 {
		return DirCandidate{}, false
	}

	return DirCandidate{Rel: entry.Name(), Markers: markers}, true
}

// isSubdirectory reports whether entry is a directory, following one level of
// symlink: DirEntry.IsDir reflects the entry's own on-disk type, so a project
// reached through a symlink would otherwise be invisible to the offer.
func isSubdirectory(dir string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}

	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}

	info, err := os.Stat(filepath.Join(dir, entry.Name()))

	return err == nil && info.IsDir()
}

// HasEnvFile reports whether the project has a .env worth asking about.
func (d Detected) HasEnvFile() bool {
	return len(d.EnvVars) > 0
}

// SuspectDir reports that the directory carries none of the usual project
// files, which is what running setup from the wrong place — the app's
// parent, most often — looks like. It is a suspicion, never a verdict: a
// valid project cannot be reliably recognized, so the absence only ever
// produces a warning and an offer, not a refusal.
func (d Detected) SuspectDir() bool {
	return len(d.RootMarkers) == 0
}

// NameListLimit caps how many names a listing prints. Past a handful they stop
// being a summary and start being a wall, and the sentence saying what to do
// about them scrolls away.
const NameListLimit = 8

// JoinNames renders a name list for a message, capped at NameListLimit with a
// count of what was left out. One implementation, because three listings in
// this feature area answer the same question and a reader should not have to
// compare them character by character to see that they agree.
func JoinNames(names []string) string {
	if len(names) <= NameListLimit {
		return strings.Join(names, ", ")
	}

	return fmt.Sprintf("%s and %d more",
		strings.Join(names[:NameListLimit], ", "), len(names)-NameListLimit)
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

	detected.RootMarkers = markersIn(dir)
	if detected.SuspectDir() {
		// The subdirectory scan runs only when there is something to suspect:
		// a directory that looks like a project needs no offer.
		detected.Candidates, detected.MoreCandidates = dirCandidates(dir)

		log.Debug("wizard: directory carries no project markers",
			"dir", dir, "candidates", len(detected.Candidates), "more", detected.MoreCandidates)
	}

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
