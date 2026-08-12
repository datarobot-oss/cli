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

// EnvFileName is the local environment file the wizard looks for. It is not
// read for values, only counted: deploys come from the manifest, and saying
// so is the point of noticing the file at all.
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
		EnvVars:       envVars(filepath.Join(dir, EnvFileName)),
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

// envVars reads and classifies a .env, nil when there is none or it cannot be
// read. The values are used here and dropped: only names and verdicts leave
// this function, so nothing downstream is holding a secret it could write out.
// The file is read a second time for order, because godotenv returns a map and
// a scrambled list would not match what the user sees in their editor.
func envVars(path string) []EnvVar {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxEnvFileSize {
		return nil
	}

	values, err := godotenv.Read(path)
	if err != nil || len(values) == 0 {
		// A .env this parser cannot read is still a .env, but listing keys we
		// are not sure of would be worse than listing none.
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	classified := make([]EnvVar, 0, len(values))
	for _, name := range orderedKeys(string(content), values) {
		classified = append(classified, ClassifyEnv(name, values[name]))
	}

	return classified
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
