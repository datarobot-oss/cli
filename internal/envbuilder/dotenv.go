// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
	PromptIndex struct {
		Prompt      UserPrompt
		PromptIndex int
		LineIndex   int
	}
	PromptIndices map[string]PromptIndex

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

	allPrompts := make(PromptIndices, len(prompts))
	// Need to add prompts that are currently missing in dotenv file separately
	missingPrompts := make(MissingPrompts, len(prompts))

	for pi, prompt := range prompts {
		// Start PromptIndex from 1 to distinguish user and prompt chunks when sorting
		if prompt.Key != "" {
			allPrompts[prompt.Key] = PromptIndex{Prompt: prompt, PromptIndex: pi + 1}
			missingPrompts[prompt.Key] = MissingPromptLineIndex{}
		} else if prompt.Env != "" {
			allPrompts[prompt.Env] = PromptIndex{Prompt: prompt, PromptIndex: pi + 1}
			missingPrompts[prompt.Env] = MissingPromptLineIndex{}
		}
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

		promptIndex, ok := allPrompts[v.Name]

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
		prompt := promptIndex.Prompt

		noPromptsYet = false

		// prompt chunks does not capture current line, it will be newly generated
		chunkString := strings.Join(lines[linesStart:l], "")

		// Remove prompt help lines from current chunk
		for _, helpLine := range prompt.HelpLines() {
			chunkString = strings.ReplaceAll(chunkString, helpLine, "")
		}

		// Save what's left as user-provided chunk
		result = append(result, Chunk{
			Lines:     chunkString,
			LineIndex: linesStart,
		})

		// Remove found prompt
		if prompt.Key != "" {
			delete(missingPrompts, prompt.Key)
		} else if prompt.Env != "" {
			delete(missingPrompts, prompt.Env)
		}

		// Advance by number of lines in user chunk
		linesStart += strings.Count(chunkString, "\n")

		// Add prompt chunk
		result = append(result, Chunk{
			Prompt:      prompt,
			PromptIndex: promptIndex.PromptIndex,
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
			Prompt:      allPrompts[missingPromptKey].Prompt,
			PromptIndex: allPrompts[missingPromptKey].PromptIndex,
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
