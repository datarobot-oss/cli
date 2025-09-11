// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type BuilderOpts struct {
	BinaryName string
	Dir        string
	Stdout     *os.File
	Stderr     *os.File
	Stdin      *os.File
}

type Builder struct{}

func NewEnvBuilder() *Builder {

	return &Builder{}
}

func (r *Builder) GatherUserPrompts(rootDir string) ([]yaml.MapSlice, error) {

	yamlFiles, err := Discover(rootDir, 2)
	if err != nil {
		return nil, fmt.Errorf("failed to discover task yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return nil, nil
	}

	var fullCollection []yaml.MapSlice

	for _, f := range yamlFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read task yaml file %s: %w", f, err)
		}
		collection := yaml.MapSlice{}
		if err = yaml.Unmarshal(data, &collection); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task yaml file %s: %w", f, err)
		}

		fullCollection = append(fullCollection, collection)

	}

	return fullCollection, nil
}
