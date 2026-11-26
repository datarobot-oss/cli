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
	"strings"
	"time"

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
	if err := checkEndpoint(req.Endpoint); err != nil {
		return nil, err
	}

	body := map[string]any{"model": req.ServedName, "messages": req.Messages, "stream": req.Stream}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	resp, err := send(ctx, http.MethodPost, joinURL(req.Endpoint, "v1/chat/completions"), req.ServedName, body, req.Stream)
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
		Usage *Usage `json:"usage"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("the model answered with something other than a chat completion: %w", err)
	}

	result := &ChatResult{Usage: parsed.Usage, Raw: raw}
	if len(parsed.Choices) > 0 {
		result.Reply = parsed.Choices[0].Message.Content
	}

	return result, nil
}

// ServedModels lists the models the endpoint serves (GET /v1/models): the
// fallback when a chat completion is refused, to tell "serves, but has no
// chat template" from "does not serve".
func ServedModels(ctx context.Context, endpoint, servedName string) ([]string, error) {
	if err := checkEndpoint(endpoint); err != nil {
		return nil, err
	}

	resp, err := send(ctx, http.MethodGet, joinURL(endpoint, "v1/models"), servedName, nil, false)
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

func checkEndpoint(endpoint string) error {
	if endpoint == "" {
		return errors.New("the workload has no endpoint yet")
	}

	same, err := drapi.URLMatchesConfiguredBase(endpoint)
	if err != nil {
		return err
	}

	if !same {
		return ErrOffHost
	}

	return nil
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

	// The URL is the workload's endpoint, checked above to be on the
	// configured DataRobot host, so the token never leaves it.
	resp, err := drapi.Do(req, timeout)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apiclient.LiftDetail(drapi.ErrFromResp(resp, u))
	}

	return resp, nil
}

// readStream reads an OpenAI server-sent event stream, writing each content
// delta to out and collecting the reply.
func readStream(r io.Reader, out io.Writer) (*ChatResult, error) {
	var reply strings.Builder

	result := &ChatResult{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}

		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}

		for _, c := range chunk.Choices {
			reply.WriteString(c.Delta.Content)

			if out != nil {
				_, _ = io.WriteString(out, c.Delta.Content)
			}
		}
	}

	result.Reply = reply.String()

	return result, scanner.Err()
}

func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + path
}

// SmokeMessages is the fixed prompt of the deploy/rollout smoke check.
func SmokeMessages() []Message {
	return []Message{{Role: "user", Content: "ping"}}
}
