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

// Package docker drives the local Docker CLI for images pushed to DataRobot:
// building a project, pushing it to the address the platform issued for an
// artifact, and pulling it back. Pushes and pulls log in with the user's
// DataRobot API token through a throwaway Docker config, so the user's own
// ~/.docker/config.json is never written and no `docker login` is needed.
package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Platform is what every image is built for: the clusters run amd64, and an
// image built natively on an arm64 laptop would not start there.
const Platform = "linux/amd64"

// InstallURL is where the error for a missing Docker sends the reader.
const InstallURL = "https://docs.docker.com/get-docker/"

// Reference is an image reference on the DataRobot registry: an address
// (Digest empty), which is where to push and runs whatever was pushed last,
// or a pinned reference, which is one exact image.
type Reference struct {
	Host       string
	ArtifactID string
	Digest     string
}

// reference is the grammar the Workload API issues: <host>/artifacts/<24 hex>
// with an optional @sha256:<64 hex>.
var reference = regexp.MustCompile(`^([^/@\s]+)/artifacts/([0-9a-f]{24})(@sha256:[0-9a-f]{64})?$`)

// ParseReference reads an image URI as a DataRobot registry reference.
func ParseReference(uri string) (Reference, bool) {
	m := reference.FindStringSubmatch(uri)
	if m == nil {
		return Reference{}, false
	}

	return Reference{Host: m[1], ArtifactID: m[2], Digest: strings.TrimPrefix(m[3], "@")}, true
}

// Address is the reference without its digest.
func (r Reference) Address() string {
	return r.Host + "/artifacts/" + r.ArtifactID
}

// runFn runs docker with args, extra environment and both output streams sent
// to out. Swapped by tests.
var runFn = func(ctx context.Context, env []string, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)

	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = out
	cmd.Stderr = out

	return cmd.Run()
}

// run is runFn with output split into lines for onLine, and a copy kept in
// capture when it is not nil.
func run(ctx context.Context, env []string, onLine func(string), capture io.Writer, args ...string) error {
	w := lineWriter(onLine)
	defer w.flush()

	var out io.Writer = w
	if capture != nil {
		out = io.MultiWriter(capture, w)
	}

	return runFn(ctx, env, out, args...)
}

// Available reports whether a Docker daemon answers, as a preflight before
// anything is created on the platform.
func Available(ctx context.Context) error {
	var out bytes.Buffer

	if err := runFn(ctx, nil, &out, "version", "--format", "{{.Server.Version}}"); err != nil {
		detail := strings.TrimSpace(out.String())
		if detail == "" {
			detail = err.Error()
		}

		return fmt.Errorf("this build mode needs a running Docker (%s). Install it from %s, start it, and run again",
			detail, InstallURL)
	}

	return nil
}

// Build builds dir as tag for Platform, with the user's own Docker config so
// private base images pull as they always do. onLine receives each line of
// output.
func Build(ctx context.Context, dir, tag string, onLine func(string)) error {
	if err := run(ctx, nil, onLine, nil,
		"build", "--platform", Platform, "--progress", "plain", "-t", tag, dir); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	return nil
}

// Push pushes ref to the registry it names, logged in with token, and
// returns the digest the registry recorded.
func Push(ctx context.Context, ref Reference, token string, onLine func(string)) (string, error) {
	var out bytes.Buffer

	err := withLogin(ref.Host, token, func(env []string) error {
		return run(ctx, env, onLine, &out, "push", ref.Address())
	})
	if err != nil {
		return "", fmt.Errorf("docker push to %s failed: %w", ref.Address(), err)
	}

	digest := parseDigest(out.String())
	if digest == "" {
		return "", fmt.Errorf("docker push to %s reported no digest", ref.Address())
	}

	return digest, nil
}

// Pull pulls ref, logged in with token, and returns the local name and the
// digest pulled. A pinned reference leaves an image with no tag, so it is also
// tagged with its address, which is the name it then answers to locally.
func Pull(ctx context.Context, ref Reference, token string, onLine func(string)) (string, string, error) {
	name := ref.Address()
	if ref.Digest != "" {
		name += "@" + ref.Digest
	}

	var out bytes.Buffer

	err := withLogin(ref.Host, token, func(env []string) error {
		return run(ctx, env, onLine, &out, "pull", "--platform", Platform, name)
	})
	if err != nil {
		return "", "", fmt.Errorf("docker pull of %s failed: %w", name, err)
	}

	digest := ref.Digest
	if digest == "" {
		digest = parseDigest(out.String())
	}

	if ref.Digest == "" {
		return name, digest, nil
	}

	if err := run(ctx, nil, onLine, nil, "tag", name, ref.Address()); err != nil {
		return "", "", fmt.Errorf("docker tag of %s failed: %w", name, err)
	}

	return ref.Address(), digest, nil
}

// withLogin runs fn with DOCKER_CONFIG pointing at a directory that holds only
// the DataRobot login, removed afterwards. Docker runs the registry's token
// exchange itself with these credentials.
func withLogin(host, token string, fn func(env []string) error) (err error) {
	if token == "" {
		return errors.New("no DataRobot API token is configured; run 'dr auth login' first")
	}

	dir, err := os.MkdirTemp("", "dr-docker-")
	if err != nil {
		return fmt.Errorf("cannot create a temporary Docker config: %w", err)
	}

	defer func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil && err == nil {
			err = fmt.Errorf("cannot remove the temporary Docker config %s, which holds your API token: %w",
				dir, rmErr)
		}
	}()

	config, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			host: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte("api:" + token))},
		},
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o600); err != nil {
		return fmt.Errorf("cannot write the temporary Docker config: %w", err)
	}

	return fn([]string{"DOCKER_CONFIG=" + dir})
}

// pushDigest is the line `docker push` ends on ("<tag>: digest: sha256:...
// size: N"), and `docker pull` prints as "Digest: sha256:...".
var pushDigest = regexp.MustCompile(`(?i)digest: (sha256:[0-9a-f]{64})`)

// parseDigest is the last digest the output printed, "" when it printed none.
func parseDigest(output string) string {
	matches := pushDigest.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return ""
	}

	return matches[len(matches)-1][1]
}

// lines hands each complete line written to it to onLine. Docker rewrites
// progress lines with carriage returns when it thinks it has a terminal;
// those end a line too, so a line never carries a half-drawn bar. exec.Cmd
// serialises writes when stdout and stderr share one writer, so no lock.
type lines struct {
	onLine func(string)
	buf    []byte
}

func lineWriter(onLine func(string)) *lines {
	return &lines{onLine: onLine}
}

func (l *lines) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)

	for {
		i := bytes.IndexAny(l.buf, "\r\n")
		if i < 0 {
			return len(p), nil
		}

		l.emit(l.buf[:i])
		l.buf = l.buf[i+1:]
	}
}

// flush delivers a last line that had no newline.
func (l *lines) flush() {
	l.emit(l.buf)
	l.buf = nil
}

func (l *lines) emit(line []byte) {
	if text := strings.TrimSpace(string(line)); text != "" && l.onLine != nil {
		l.onLine(text)
	}
}
