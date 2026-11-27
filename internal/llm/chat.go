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

package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/apiclient"
)

// ModelHeader is the AI gateway's routing header. The gateway authenticates
// before it reads the body, so a request without it is refused even with a
// valid token; workload-api's own llm readiness probe sends it too.
const ModelHeader = "x-ai-eg-model"

// chatTimeout bounds one non-streaming completion and streamTimeout one
// stream; the caller's context (Ctrl-C) ends either sooner.
const (
	chatTimeout   = 5 * time.Minute
	streamTimeout = 30 * time.Minute
)

// Message is one OpenAI chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is what `dr llm chat` and the smoke check send.
type ChatRequest struct {
	Endpoint   string
	ServedName string
	Messages   []Message
	MaxTokens  int
	Stream     bool
}

// Usage is the token accounting of a completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResult is a completion: the reply, its usage, and the raw response for
// --output-format json (nil when streamed).
type ChatResult struct {
	Reply string
	Usage *Usage
	Raw   json.RawMessage
}

// ErrOffHost marks an endpoint on another host than the configured
// DataRobot API: an Enclave's own Agent Router. The DataRobot token is not
// sent there.
var ErrOffHost = errors.New("the workload's endpoint is on another host than this DataRobot installation (an Enclave); chat supports workloads served through this installation for now")

// Chat sends one chat completion to the workload's advertised endpoint, with
// the request shape workload-api's llm readiness gate uses. With Stream, each
// content delta is written to out as it arrives.
func Chat(ctx context.Context, req ChatRequest, out io.Writer) (*ChatResult, error) {
	u, err := routeURL(req.Endpoint, "v1/chat/completions")
	if err != nil {
		return nil, err
	}

	body := map[string]any{"model": req.ServedName, "messages": req.Messages, "stream": req.Stream}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	resp, err := send(ctx, http.MethodPost, u, req.ServedName, body, req.Stream)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if req.Stream {
		return readStream(resp.Body, out)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage *Usage          `json:"usage"`
		Error json.RawMessage `json:"error"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("the model answered with something other than a chat completion: %w", err)
	}

	// A 2xx without a choice is not an answer: an error object, {} or null
	// must not let the smoke check report that the model answered. An empty
	// content string in a real choice is a legitimate answer.
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("the model returned no completion: %s", snippet(raw))
	}

	return &ChatResult{Reply: parsed.Choices[0].Message.Content, Usage: parsed.Usage, Raw: raw}, nil
}

// ServedModels lists the models the endpoint serves (GET /v1/models): the
// fallback when a chat completion is refused, to tell "serves, but has no
// chat template" from "does not serve".
func ServedModels(ctx context.Context, endpoint, servedName string) ([]string, error) {
	u, err := routeURL(endpoint, "v1/models")
	if err != nil {
		return nil, err
	}

	resp, err := send(ctx, http.MethodGet, u, servedName, nil, false)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		ids = append(ids, m.ID)
	}

	return ids, nil
}

// workloadIngressPrefix is the only path that receives the DataRobot token,
// as in `dr workload up`'s endpoint check: other URLs on the same host can be
// direct service URLs that end in user-owned containers.
const workloadIngressPrefix = config.DRAPIURLSuffix + "/endpoints/workloads/"

// ErrNotIngress marks an endpoint on this installation's host that is not
// its public workload ingress, so it is not sent the DataRobot token.
var ErrNotIngress = errors.New("the workload's endpoint is not this installation's public workload ingress (" + workloadIngressPrefix + "), so the DataRobot token is not sent to it")

// Chattable reports whether dr llm chat sends requests to the endpoint: this
// installation's public workload ingress. An Enclave's own URL is not.
func Chattable(endpoint string) bool {
	_, err := routeURL(endpoint, "v1/models")

	return err == nil
}

// routeURL checks the endpoint may receive the DataRobot token (this
// installation's host and its public workload ingress path, with no dot
// segments) and joins route onto its path, keeping its query (a ?protonId=
// pins the generation) where it belongs.
func routeURL(endpoint, route string) (string, error) {
	if endpoint == "" {
		return "", errors.New("the workload has no endpoint yet")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint URL %q: %w", endpoint, err)
	}

	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("endpoint URL %q is not absolute", endpoint)
	}

	same, err := drapi.URLMatchesConfiguredBase(endpoint)
	if err != nil {
		return "", err
	}

	if !same {
		return "", ErrOffHost
	}

	if !strings.HasPrefix(u.EscapedPath(), workloadIngressPrefix) || climbsOut(u.EscapedPath()) {
		return "", ErrNotIngress
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + route
	u.RawPath = ""

	return u.String(), nil
}

// maxUnescapes bounds how many layers of percent-encoding climbsOut peels; a
// path still changing after that many is refused rather than trusted.
const maxUnescapes = 3

// climbsOut reports whether the path has a dot segment under any layer of
// percent-encoding (%2e%2e, %252e%252e), with a backslash counted as a
// separator: a proxy in front of the ingress may decode and resolve either,
// which would carry the token to another path on the host. Same rule as the
// endpoint check in dr workload up.
func climbsOut(path string) bool {
	for range maxUnescapes {
		if slices.ContainsFunc(strings.FieldsFunc(path, isPathSeparator), isDotSegment) {
			return true
		}

		decoded, err := url.PathUnescape(path)
		if err != nil {
			return true
		}

		if decoded == path {
			return false
		}

		path = decoded
	}

	return true
}

func isPathSeparator(r rune) bool {
	return r == '/' || r == '\\'
}

func isDotSegment(segment string) bool {
	return segment == "." || segment == ".."
}

func snippet(raw []byte) string {
	const maxLen = 200

	s := strings.TrimSpace(string(raw))
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}

	if s == "" {
		return "(empty body)"
	}

	return s
}

func send(ctx context.Context, method, u, servedName string, body any, stream bool) (*http.Response, error) {
	var reader io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}

	if err := drapi.AuthorizeRequest(req); err != nil { //nolint:contextcheck // drapi takes no context; the request itself carries ctx
		return nil, err
	}

	req.Header.Set(ModelHeader, servedName)
	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// drapi treats a zero timeout as its short default, which would cut a
	// long stream; a stream gets a generous bound and Ctrl-C ends it sooner.
	timeout := chatTimeout
	if stream {
		timeout = streamTimeout
	}

	// The URL is the workload's endpoint, checked by routeURL to be this
	// installation's public workload ingress. Redirects are not followed: a
	// 307/308 would replay the token and the prompt to a URL routeURL never
	// checked, so a redirect is reported as the 3xx it is.
	client := drapi.NewHTTPClient(timeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apiclient.LiftDetail(drapi.ErrFromResp(resp, u))
	}

	return resp, nil
}

// readStream reads an OpenAI server-sent event stream, writing each content
// delta to out and collecting the reply. An error event, a malformed event,
// or a stream that ends without [DONE] or a finish_reason is an error: a
// truncated reply must not read as an answer.
func readStream(r io.Reader, out io.Writer) (*ChatResult, error) {
	var reply strings.Builder

	result := &ChatResult{}
	finished := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}

		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			finished = true

			break
		}

		done, err := readEvent(data, result, &reply, out)
		if err != nil {
			return nil, err
		}

		finished = finished || done
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if !finished {
		return nil, errors.New("the stream ended before the model finished its reply")
	}

	result.Reply = reply.String()

	return result, nil
}

// readEvent applies one stream event and reports whether it finished the
// reply (a finish_reason).
func readEvent(data string, result *ChatResult, reply *strings.Builder, out io.Writer) (bool, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage          `json:"usage"`
		Error json.RawMessage `json:"error"`
	}

	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false, fmt.Errorf("the stream sent a malformed event: %s", snippet([]byte(data)))
	}

	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return false, fmt.Errorf("the model reported an error mid-stream: %s", snippet(chunk.Error))
	}

	if chunk.Usage != nil {
		result.Usage = chunk.Usage
	}

	finished := false

	for _, c := range chunk.Choices {
		reply.WriteString(c.Delta.Content)

		if out != nil {
			_, _ = io.WriteString(out, c.Delta.Content)
		}

		if c.FinishReason != nil && *c.FinishReason != "" {
			finished = true
		}
	}

	return finished, nil
}

// SmokeMessages is the fixed prompt of the deploy/rollout smoke check.
func SmokeMessages() []Message {
	return []Message{{Role: "user", Content: "ping"}}
}
