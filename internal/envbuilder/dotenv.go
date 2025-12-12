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
	void struct{}

	PromptIndex struct {
		Prompt      UserPrompt
		PromptIndex int
		LineIndex   int
	}
	PromptsIndexMap   map[string]PromptIndex
	MissingPromptsMap map[string]void

	Chunk struct {
		Prompt      UserPrompt
		PromptIndex int
		Lines       string
		LineIndex   int
	}

	DotenvChunks []Chunk
)

func mergedDotenvChunks(prompts []UserPrompt, contents string) DotenvChunks {
	result := make(DotenvChunks, 0)

	allPrompts := make(PromptsIndexMap, len(prompts))
	// Prompts that are currently missing in dotenv file
	missingPrompts := make(MissingPromptsMap, len(prompts))

	for pi, prompt := range prompts {
		// Start PromptIndex from 1 to distinguish user and prompt chunks when sorting
		if prompt.Key != "" {
			allPrompts[prompt.Key] = PromptIndex{Prompt: prompt, PromptIndex: pi + 1}
			missingPrompts[prompt.Key] = void{}
		} else if prompt.Env != "" {
			allPrompts[prompt.Env] = PromptIndex{Prompt: prompt, PromptIndex: pi + 1}
			missingPrompts[prompt.Env] = void{}
		}
	}

	unquotedValues, _ := godotenv.Unmarshal(contents)
	lines := slices.Collect(strings.Lines(contents))
	linesStart := 0

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

			// Proceed to next line
			continue
		}

		// Prompt managed by cli
		prompt := promptIndex.Prompt

		// prompt chunks does not capture current line, it will be newly generated
		chunkString := strings.Join(lines[linesStart:l], "")

		// Remove prompt help lines from current chunk
		for _, helpLine := range prompt.HelpLines() {
			chunkString = strings.Replace(chunkString, helpLine, "", -1)
		}

		// Save what's left as user-provided chunk
		result = append(result, Chunk{
			Lines:     chunkString,
			LineIndex: linesStart,
		})

		// Advance by number of lines in user chunk
		linesStart += strings.Count(chunkString, "\n")

		// Remove found prompt
		if prompt.Key != "" {
			delete(missingPrompts, prompt.Key)
		} else if prompt.Env != "" {
			delete(missingPrompts, prompt.Env)
		}

		// Add prompt chunk
		result = append(result, Chunk{
			Prompt:      prompt,
			PromptIndex: promptIndex.PromptIndex,
			LineIndex:   linesStart,
		})

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
		missingPrompt := allPrompts[missingPromptKey]
		missingPromptChunk := Chunk{
			Prompt:      missingPrompt.Prompt,
			PromptIndex: missingPrompt.PromptIndex,
		}
		result = append(result, missingPromptChunk)
	}

	return result
}

func (ch DotenvChunks) Sort() DotenvChunks {
	slices.SortFunc(ch, func(a, b Chunk) int {
		if a.PromptIndex == 0 && b.PromptIndex == 0 {
			return cmp.Compare(a.LineIndex, b.LineIndex)
		}

		if a.PromptIndex != 0 && b.PromptIndex != 0 {
			return cmp.Compare(a.PromptIndex, b.PromptIndex)
		}

		return 0
		// return cmp.Compare(a.LineIndex, b.LineIndex)
		// return cmp.Compare(a.PromptIndex, b.PromptIndex)
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
			// result.WriteString("chunk.Lines")
			result.WriteString(chunk.Lines)
			// result.WriteString("chunk.Lines")
		}
	}

	return result.String()
}
