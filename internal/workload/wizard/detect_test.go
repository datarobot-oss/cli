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
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDockerfile puts a Dockerfile in dir and returns dir, so a test reads
// as the project it is describing.
func writeDockerfile(t *testing.T, dir, content string) string {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, DockerfileName), []byte(content), 0o600))

	return dir
}

func TestDetect_NoDockerfile(t *testing.T) {
	detected := Detect(t.TempDir())

	assert.False(t, detected.HasDockerfile)
	assert.Equal(t, manifest.DefaultPort, detected.Port)
	assert.NotContains(t, detected.PortSource, "EXPOSE")
}

func TestDetect_ReadsExpose(t *testing.T) {
	tests := map[string]struct {
		dockerfile string
		wantPort   int
		wantSource bool
	}{
		"plain":            {"FROM scratch\nEXPOSE 3000\n", 3000, true},
		"with protocol":    {"FROM scratch\nEXPOSE 9000/tcp\n", 9000, true},
		"lowercase":        {"FROM scratch\nexpose 5000\n", 5000, true},
		"indented":         {"FROM scratch\n  EXPOSE 7000\n", 7000, true},
		"first of several": {"FROM scratch\nEXPOSE 3000\nEXPOSE 9090\n", 3000, true},
		// A build-time variable says nothing we can act on, so the default
		// stands rather than a guess.
		"build arg":  {"FROM scratch\nEXPOSE $PORT\n", manifest.DefaultPort, false},
		"no expose":  {"FROM scratch\nCMD [\"/app\"]\n", manifest.DefaultPort, false},
		"bare word":  {"FROM scratch\nEXPOSE\n", manifest.DefaultPort, false},
		"not a port": {"FROM scratch\nEXPOSE http\n", manifest.DefaultPort, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			detected := Detect(writeDockerfile(t, t.TempDir(), test.dockerfile))

			assert.True(t, detected.HasDockerfile)
			assert.Equal(t, test.wantPort, detected.Port)

			if test.wantSource {
				assert.Contains(t, detected.PortSource, "EXPOSE")
			} else {
				assert.NotContains(t, detected.PortSource, "EXPOSE")
			}
		})
	}
}

// The suggested name has to be usable as typed, because the happy path is
// pressing Enter through it.
func TestDetect_SuggestsANameFromTheDirectory(t *testing.T) {
	tests := map[string]string{
		"my-app":        "my-app",
		"My App":        "my-app",
		"my.app.v2":     "my-app-v2",
		"_private_":     "private",
		"señor-café":    "seor-caf",
		"...":           "my-app",
		"UPPERCASE":     "uppercase",
		"trailing-dash": "trailing-dash",
	}

	for dirName, want := range tests {
		t.Run(dirName, func(t *testing.T) {
			// The directory is deliberately not created. The name comes from
			// the path, so nothing here needs one to exist, and "..." is a
			// name Windows refuses to create: it strips trailing dots, so the
			// path resolves to the parent and the mkdir fails as EEXIST.
			assert.Equal(t, want, Detect(filepath.Join(t.TempDir(), dirName)).Name)
		})
	}
}

// Only the key names are read. The values are the user's secrets and have no
// business in a wizard that is not importing them.
func TestDetect_ReadsEnvKeyNamesInOrder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName),
		[]byte("# a comment\nLOG_LEVEL=debug\nOPENAI_API_KEY=sk-secret\n\nexport DATABASE_URL=postgres://x\n"), 0o600))

	detected := Detect(dir)

	names := make([]string, 0, len(detected.EnvVars))
	for _, v := range detected.EnvVars {
		names = append(names, v.Name)
	}

	assert.Equal(t, []string{"LOG_LEVEL", "OPENAI_API_KEY", "DATABASE_URL"}, names,
		"order matches the file, and export is not part of the name")
	assert.True(t, detected.HasEnvFile())
}

func TestDetect_NoEnvFileIsSilent(t *testing.T) {
	detected := Detect(t.TempDir())

	assert.Empty(t, detected.EnvVars)
	assert.False(t, detected.HasEnvFile())
}

func TestDetect_EmptyEnvFileIsSilent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("# nothing set yet\n"), 0o600))

	assert.False(t, Detect(dir).HasEnvFile())
}

func TestDetect_SingularKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("ONLY=one\n"), 0o600))

	require.Len(t, Detect(dir).EnvVars, 1)
}

// writeEnv puts a .env in dir, which is all Detect needs to try parsing it.
func writeEnv(t *testing.T, dir, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte(content), 0o600))
}

// godotenv reports the whole unparsed remainder of the input as part of its
// message, so an unfiltered wrap would print every value below the bad line.
// The reason and the position are worth showing; nothing else from the file
// is, and a headless run puts this straight into a build log.
func TestEnvVars_ParseErrorNamesTheLineWithoutTheFile(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, "GOOD=1\nBAD-NAME=x\nAPI_TOKEN=sk-live-do-not-print\nDB_PASSWORD=hunter2\n")

	detected := Detect(dir)

	require.Error(t, detected.EnvErr)
	assert.Empty(t, detected.EnvVars)

	msg := detected.EnvErr.Error()

	assert.Contains(t, msg, "unexpected character")
	assert.Contains(t, msg, "on line 2")
	assert.NotContains(t, msg, "sk-live-do-not-print")
	assert.NotContains(t, msg, "hunter2")
	assert.NotContains(t, msg, "API_TOKEN")
}

// The blank lines matter: the line number is counted from the file, not from
// the variables in it.
func TestEnvVars_ParseErrorCountsBlankLines(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, "A=1\n\n# a comment\n\nB-C=3\nZ=sk-tail\n")

	detected := Detect(dir)

	require.Error(t, detected.EnvErr)
	assert.Contains(t, detected.EnvErr.Error(), "on line 5")
	assert.NotContains(t, detected.EnvErr.Error(), "sk-tail")
}

// This is the message that ends in the raw value rather than a quoted one,
// so it is reported by class and never quoted back.
func TestEnvVars_UnterminatedQuoteIsNotEchoed(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, "TOKEN=\"unclosed-sk-live-secret\nNEXT=2\n")

	detected := Detect(dir)

	require.Error(t, detected.EnvErr)
	assert.Contains(t, detected.EnvErr.Error(), "quoted value is never closed")
	assert.NotContains(t, detected.EnvErr.Error(), "sk-live-secret")
}

// A value carrying the separator the other branch splits on must not reach
// that branch: matching by opening rather than by substring is what stops
// half the value being printed.
func TestParseProblem_UnterminatedQuoteWinsOverTheNearSplit(t *testing.T) {
	err := errors.New(`unterminated quoted value "sk near live-secret`)

	assert.Equal(t, "a quoted value is never closed", parseProblem(err, ""))
}

// An unrecognized message cannot be assumed free of file content, so none of
// it is repeated.
func TestParseProblem_UnknownMessageIsDescribedGenerically(t *testing.T) {
	err := errors.New("some future parser complaint about sk-live-secret")

	assert.Equal(t, "it is not in KEY=value form", parseProblem(err, ""))
}

// A payload that is not a suffix of the file yields no line rather than a
// wrong one.
func TestParseProblem_UnrecognizablePayloadYieldsNoLine(t *testing.T) {
	err := errors.New(`unexpected character "-" in variable name near "not-from-this-file"`)

	assert.Equal(t, `unexpected character "-" in variable name`, parseProblem(err, "A=1\n"))
}

// A directory with none of the usual project files is suspect, and the offer
// is its marker-bearing subdirectories: hidden ones are never the project,
// empty ones say nothing, and one already holding a manifest is entered with
// --dir rather than set up afresh.
func TestDetect_SuspectDirOffersItsProjectLookingSubdirectories(t *testing.T) {
	dir := t.TempDir()

	app := filepath.Join(dir, "my-app")
	require.NoError(t, os.MkdirAll(app, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(app, "pyproject.toml"), []byte("[project]\n"), 0o644))

	hidden := filepath.Join(dir, ".cache")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hidden, "Dockerfile"), []byte("FROM scratch\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))

	configured := filepath.Join(dir, "configured")
	require.NoError(t, os.MkdirAll(configured, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configured, "go.mod"), []byte("module x\n"), 0o644))
	require.NoError(t, os.WriteFile(manifest.Path(configured), []byte("name: x\n"), 0o644))

	detected := Detect(dir)

	assert.True(t, detected.SuspectDir())
	require.Len(t, detected.Candidates, 1)
	assert.Equal(t, "my-app", detected.Candidates[0].Rel)
	assert.Equal(t, []string{"Dockerfile", "pyproject.toml"}, detected.Candidates[0].Markers)
}

// A directory carrying any project file is not suspect, and the subdirectory
// scan never runs: a directory that looks like a project needs no offer.
func TestDetect_MarkerBearingDirIsNotSuspect(t *testing.T) {
	detected := Detect(writeDockerfile(t, t.TempDir(), "FROM scratch\n"))

	assert.False(t, detected.SuspectDir())
	assert.Contains(t, detected.RootMarkers, "Dockerfile")
	assert.Empty(t, detected.Candidates)
}
