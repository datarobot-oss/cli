// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package envbuilder

import (
	"cmp"
	"slices"
	"strings"

	"github.com/joho/godotenv"
)

func DotenvFromPrompts(prompts []UserPrompt) string {
	var result strings.Builder

	for _, prompt := range prompts {
		if prompt.SkipSaving() {
			continue
		}

		result.WriteString(prompt.String())
		result.WriteString("\n")
	}

	return result.String()
}

func DefaultDotenvFile() string {
	return DotenvFromPrompts(corePrompts)
}

func DotenvFromPromptsMerged(prompts []UserPrompt, contents string) string {
	chunks := mergedDotenvChunks(prompts, contents)

	chunks.Sort()

	return chunks.String()
}

type (
	PromptInstance struct {
		Prompt      UserPrompt
		PromptIndex int
		HelpLines   []string
	}
	PromptInstances map[string]PromptInstance

	MissingPromptLineIndex struct {
		LineIndex int
	}
	MissingPrompts map[string]MissingPromptLineIndex

	Chunk struct {
		Prompt      UserPrompt
		PromptIndex int
		Lines       string
		LineIndex   int
	}

	DotenvChunks []Chunk
)

// mergedDotenvChunks walks dotenv file line-by-line, grouping lines into three chunk types:
// - variables backed by prompts (can be commented)
// - user-provided variables (not commented)
// - everything else (comments, empty lines, etc.)
//
// help comments for prompt-backed variables are split from user-provided comments and discarded
// they are added later from UserPrompt struct value
//
// returns slice of chunks of dotenv file with their position
func mergedDotenvChunks(prompts []UserPrompt, contents string) DotenvChunks { //nolint: cyclop
	result := make(DotenvChunks, 0)

	promptInstances := make(PromptInstances, len(prompts))
	// Need to add prompts that are currently missing in dotenv file separately
	missingPrompts := make(MissingPrompts, len(prompts))

	for pi, prompt := range prompts {
		varName := prompt.VarName()

		if promptInstance, ok := promptInstances[varName]; ok {
			// Found prompt with duplicated name
			if prompt.Active {
				promptInstance.Prompt = prompt
				promptInstance.PromptIndex = pi + 1
			}

			// Help lines in dotenv file might come from different duplicate, need to clean them all
			promptInstance.HelpLines = append(promptInstance.HelpLines, prompt.HelpLines()...)

			promptInstances[varName] = promptInstance
		} else {
			// Start PromptIndex from 1 to distinguish user and prompt chunks when sorting
			promptInstances[varName] = PromptInstance{
				Prompt:      prompt,
				PromptIndex: pi + 1,
				HelpLines:   prompt.HelpLines(),
			}
		}

		missingPrompts[varName] = MissingPromptLineIndex{}
	}

	unquotedValues, _ := godotenv.Unmarshal(contents)
	lines := slices.Collect(strings.Lines(contents))
	linesStart := 0
	noPromptsYet := true

	for l := 0; l < len(lines); l++ {
		line := lines[l]

		v := NewFromLine(line, unquotedValues)

		// Proceed to next line if current line is not a variable
		if v.Name == "" {
			continue
		}

		promptInstance, ok := promptInstances[v.Name]

		// If user-provided variable
		if !ok {
			// Create new chunk, including current line
			result = append(result, Chunk{
				Lines:     strings.Join(lines[linesStart:l+1], ""),
				LineIndex: linesStart,
			})

			// Start new chunk at next line
			linesStart = l + 1

			// put prompts at the end of file if only user variables are present in dotenv file
			if noPromptsYet {
				for missingPromptKey := range missingPrompts {
					missingPrompts[missingPromptKey] = MissingPromptLineIndex{
						LineIndex: linesStart,
					}
				}
			}

			// Proceed to next line
			continue
		}

		// Prompt managed by cli
		prompt := promptInstance.Prompt

		noPromptsYet = false

		// prompt chunks does not capture current line, it will be newly generated
		chunkString := strings.Join(lines[linesStart:l], "")

		// Remove prompt help lines from current chunk
		for _, helpLine := range promptInstance.HelpLines {
			chunkString = strings.ReplaceAll(chunkString, helpLine, "")
		}

		// Save what's left as user-provided chunk
		result = append(result, Chunk{
			Lines:     chunkString,
			LineIndex: linesStart,
		})

		// Remove found prompt
		delete(missingPrompts, prompt.VarName())

		// Advance by number of lines in user chunk
		linesStart += strings.Count(chunkString, "\n")

		// Add prompt chunk
		result = append(result, Chunk{
			Prompt:      prompt,
			PromptIndex: promptInstance.PromptIndex,
			LineIndex:   linesStart,
		})

		for missingPromptKey := range missingPrompts {
			missingPrompts[missingPromptKey] = MissingPromptLineIndex{
				// Put missing and present prompts near each other
				LineIndex: linesStart,
			}
		}

		// Start new chunk at next line
		linesStart = l + 1

		// For multiline values advance by number of extra lines
		if valueLinesCount := strings.Count(v.Value, "\n"); valueLinesCount > 0 {
			l += valueLinesCount - 1
			linesStart += valueLinesCount - 1
		}
	}

	// Add prompt chunks that were missing in dotenv file
	for missingPromptKey := range missingPrompts {
		result = append(result, Chunk{
			Prompt:      promptInstances[missingPromptKey].Prompt,
			PromptIndex: promptInstances[missingPromptKey].PromptIndex,
			LineIndex:   missingPrompts[missingPromptKey].LineIndex,
		})
	}

	return result
}

// Sort sorts by chunk position in dotenv file
func (ch DotenvChunks) Sort() DotenvChunks {
	slices.SortFunc(ch, func(a, b Chunk) int {
		// If both are prompt chunks sort by position in prompts array
		if a.PromptIndex != 0 && b.PromptIndex != 0 {
			return cmp.Compare(a.PromptIndex, b.PromptIndex)
		}

		// Otherwise sort by position in dotenv file
		return cmp.Compare(a.LineIndex, b.LineIndex)
	})

	return ch
}

func (ch DotenvChunks) String() string {
	var result strings.Builder

	for _, chunk := range ch {
		if chunk.PromptIndex > 0 {
			prompt := chunk.Prompt

			if prompt.SkipSaving() {
				continue
			}

			result.WriteString(prompt.String())
			result.WriteString("\n")
		} else {
			result.WriteString(chunk.Lines)
		}
	}

	return result.String()
}
