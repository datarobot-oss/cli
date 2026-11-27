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

package chat

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		system       string
		maxTokens    int
		stream       bool
	)

	cmd := &cobra.Command{
		Use:   "chat <workload-id> [prompt]",
		Short: "Send a chat request to a deployed LLM.",
		Long: `Send one chat completion to an llm workload, at the endpoint workload-api
advertises for it (what 'dr workload endpoint' prints), as the model clients
call it: the artifact's served name.

The prompt is the second argument, or stdin when it is omitted. Text output
prints the reply and the token usage; --stream prints the reply as it is
generated; --output-format json prints the raw completion.

When the model refuses a chat request (400/422), the command lists the models
the endpoint serves, to tell a model with no chat template (an embedding
model, for example) from one that is not served.

Example:
  dr llm chat 68b0c1d2e3f4a5b6c7d8e9f0 "Say hello in one word."
  echo "Summarise Dynamo in a sentence" | dr llm chat 68b0c1d2e3f4a5b6c7d8e9f0 --stream
  dr llm chat 68b0c1d2e3f4a5b6c7d8e9f0 "Hi" --system "Answer in French" --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			prompt, err := readPrompt(cmd, args)
			if err != nil {
				return err
			}

			if stream && outputFormat == outputformat.OutputFormatJSON {
				return errors.New("--stream prints text as it arrives: drop --output-format json, or --stream")
			}

			target, err := llm.ResolveTarget(args[0])
			if err != nil {
				return err
			}

			req := llm.ChatRequest{
				Endpoint:   target.Endpoint,
				ServedName: target.ServedName,
				Messages:   messages(system, prompt),
				MaxTokens:  maxTokens,
				Stream:     stream,
			}

			var live io.Writer
			if stream {
				live = cmd.OutOrStdout()
			}

			res, err := llm.Chat(cmd.Context(), req, live)
			if err != nil {
				return explainRefusal(cmd, target, err)
			}

			return render(cmd, outputFormat, res, stream)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().StringVar(&system, "system", "", "System message")
	cmd.Flags().Var(countflags.PositiveInt(&maxTokens, 256), "max-tokens", "Maximum tokens to generate")
	cmd.Flags().BoolVar(&stream, "stream", false, "Print the reply as it is generated")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"system":        system != "",
			"stream":        stream,
			"max_tokens":    maxTokens,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func readPrompt(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 2 {
		return args[1], nil
	}

	if reader.IsStdinTerminal() {
		return "", errors.New("no prompt: pass it as the second argument or on stdin")
	}

	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", err
	}

	prompt := strings.TrimSpace(string(b))
	if prompt == "" {
		return "", errors.New("the prompt on stdin is empty")
	}

	return prompt, nil
}

func messages(system, prompt string) []llm.Message {
	var msgs []llm.Message

	if system != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: system})
	}

	return append(msgs, llm.Message{Role: "user", Content: prompt})
}

func render(cmd *cobra.Command, format outputformat.OutputFormat, res *llm.ChatResult, streamed bool) error {
	out := cmd.OutOrStdout()

	if format == outputformat.OutputFormatJSON {
		_, err := out.Write(append(res.Raw, '\n'))

		return err
	}

	if streamed {
		fmt.Fprintln(out)
	} else {
		fmt.Fprintln(out, res.Reply)
	}

	if res.Usage != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "\n(%d prompt + %d completion tokens)\n", res.Usage.PromptTokens, res.Usage.CompletionTokens)
	}

	return nil
}

// explainRefusal adds, for a 400/422, whether the endpoint serves the model
// at all: a model with no chat template (an embedding model) is served but
// refuses chat requests.
func explainRefusal(cmd *cobra.Command, t *llm.Target, err error) error {
	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	if httpErr.StatusCode != http.StatusBadRequest && httpErr.StatusCode != http.StatusUnprocessableEntity {
		return err
	}

	served, listErr := llm.ServedModels(cmd.Context(), t.Endpoint, t.ServedName)
	if listErr != nil {
		return err
	}

	if slices.Contains(served, t.ServedName) {
		return fmt.Errorf("%w\nThe endpoint serves %q but refused a chat request: the model may have no chat template (an embedding model, for example)", err, t.ServedName)
	}

	return fmt.Errorf("%w\nThe endpoint does not list %q among the models it serves (%s)", err, t.ServedName, strings.Join(served, ", "))
}
