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

package envbuilder

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/log"
	"gopkg.in/yaml.v3"
)

const (
	fieldEnv     = "env"
	fieldKey     = "key"
	fieldHelp    = "help"
	yamlMapping  = "mapping"
	yamlSequence = "sequence"
)

// ParsedYaml represents the structure of a prompt definition YAML file,
// mapping section names to lists of prompts.
type ParsedYaml map[string][]UserPrompt

// PromptFileSchema validates that a YAML file conforms to the prompt definition schema:
// - Root must be a mapping (sections)
// - Each section value must be a sequence of mappings (prompts)
// - Each prompt must have at least one of: env or key
// - Each prompt must have a help field
type PromptFileSchema struct{}

// Validate checks if the provided YAML node conforms to the prompt file schema.
// Returns nil if valid, or an error describing validation failures.
func (s *PromptFileSchema) Validate(doc *yaml.Node) error {
	if doc == nil {
		return errors.New("document is empty")
	}

	if err := s.validateRootMapping(doc); err != nil {
		return err
	}

	if err := s.validateSections(doc); err != nil {
		return err
	}

	return nil
}

// validateRootMapping ensures the root is a mapping with content.
func (s *PromptFileSchema) validateRootMapping(doc *yaml.Node) error {
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("root must be a %s (sections), got %v", yamlMapping, doc.Kind)
	}

	if len(doc.Content) == 0 {
		return errors.New("root mapping is empty")
	}

	return nil
}

// validateSections validates that each section contains a sequence of prompt mappings.
func (s *PromptFileSchema) validateSections(doc *yaml.Node) error {
	// mapping content alternates key, value, key, value...
	// validate that every value is a sequence of mappings (prompts)
	for i := 1; i < len(doc.Content); i += 2 {
		sectionValue := doc.Content[i]
		if sectionValue.Kind != yaml.SequenceNode {
			return fmt.Errorf("section value must be a %s (prompts), got %v", yamlSequence, sectionValue.Kind)
		}

		if err := s.validatePrompts(sectionValue.Content); err != nil {
			return err
		}
	}

	return nil
}

// validatePrompts validates each prompt in a section.
func (s *PromptFileSchema) validatePrompts(promptNodes []*yaml.Node) error {
	for idx, promptNode := range promptNodes {
		if promptNode.Kind != yaml.MappingNode {
			return fmt.Errorf("prompt at index %d must be a %s, got %v", idx, yamlMapping, promptNode.Kind)
		}

		if err := s.validatePromptYaml(promptNode); err != nil {
			return fmt.Errorf("invalid prompt at index %d: %w", idx, err)
		}
	}

	return nil
}

// validatePromptYaml validates a single prompt mapping node against the schema.
// A valid prompt must have at least one of: env or key, and should have help.
func (s *PromptFileSchema) validatePromptYaml(promptNode *yaml.Node) error {
	if len(promptNode.Content) == 0 {
		return errors.New("prompt mapping is empty")
	}

	fields := s.extractPromptFields(promptNode)

	if !fields.hasEnv && !fields.hasKey {
		return errors.New("prompt must have either 'env' or 'key' field")
	}

	if !fields.hasHelp {
		// TODO should UserPrompt.Help be required? Check in all existing templates
		// to see if any are missing it, and if so, change this back to return error
		// in a follow-up PR. For now, just log a warning.
		log.Info("prompt is missing recommended 'help' field for user guidance")
	}

	return nil
}

// promptFields tracks which fields are present in a prompt.
type promptFields struct {
	hasEnv  bool
	hasKey  bool
	hasHelp bool
}

// extractPromptFields extracts field presence from a prompt mapping node.
func (s *PromptFileSchema) extractPromptFields(promptNode *yaml.Node) promptFields {
	fields := promptFields{}

	// mapping content alternates key, value, key, value...
	for i := 0; i < len(promptNode.Content); i += 2 {
		keyNode := promptNode.Content[i]
		fieldName := keyNode.Value

		switch fieldName {
		case fieldEnv:
			fields.hasEnv = true
		case fieldKey:
			fields.hasKey = true
		case fieldHelp:
			fields.hasHelp = true
		}
	}

	return fields
}

// UnmarshalPromptFile unmarshals YAML data into a ParsedYaml map.
func UnmarshalPromptFile(data []byte) (ParsedYaml, error) {
	var fileParsed ParsedYaml

	if err := yaml.Unmarshal(data, &fileParsed); err != nil {
		return nil, err
	}

	return fileParsed, nil
}

// ValidateAndSkipNonPromptFiles validates YAML content and returns whether it's a valid prompt file.
// Non-conforming files are logged as debug messages and skipped silently.
// This avoids errors on copier answer files, version manifests, and other
// YAML that happens to live under .datarobot/.
func ValidateAndSkipNonPromptFiles(data []byte) bool {
	var root yaml.Node

	if err := yaml.Unmarshal(data, &root); err != nil {
		// malformed YAML: let the real unmarshal surface the error
		return true
	}

	if len(root.Content) == 0 {
		return false
	}

	schema := &PromptFileSchema{}
	if err := schema.Validate(root.Content[0]); err != nil {
		return false
	}

	return true
}
