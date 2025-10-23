// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestTemplateTestSuite(t *testing.T) {
	suite.Run(t, new(TemplateTestSuite))
}

type TemplateTestSuite struct {
	suite.Suite
	tempDir  string
	dotfile  string
	template string
}

func (suite *TemplateTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-template-test")
	suite.tempDir = dir

	suite.dotfile = filepath.Join(suite.tempDir, ".env")
	suite.template = filepath.Join(suite.tempDir, ".env.template")

	suite.T().Setenv("DATAROBOT_ENDPOINT", "")
	suite.T().Setenv("DATAROBOT_ENDPOINT_SHORT", "")
	suite.T().Setenv("DATAROBOT_API_TOKEN", "")
}

func (suite *TemplateTestSuite) TestCreateDotenvWithoutTemplate() {
	_, contents, dotenvTemplateUsed, err := writeUsingTemplateFile(suite.dotfile)

	suite.NoError(err) //nolint: testifylint

	suite.FileExists(suite.dotfile, "Expected dotenv file to be created")

	suite.Equal(
		"DATAROBOT_ENDPOINT=\nDATAROBOT_API_TOKEN=\n\nUSE_DATAROBOT_LLM_GATEWAY=\n",
		contents,
	)
	suite.Empty(dotenvTemplateUsed)

	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Verify header format with timestamp
	suite.Regexp(`# Edited using .dr dotenv. on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)
	suite.Contains(content, "DATAROBOT_ENDPOINT=\nDATAROBOT_API_TOKEN=\n\nUSE_DATAROBOT_LLM_GATEWAY=\n")
}

func (suite *TemplateTestSuite) TestCreateDotenvWithTemplate() {
	_ = os.WriteFile(suite.template, []byte("USE_DATAROBOT_LLM_GATEWAY=\n"), 0o644)

	suite.FileExists(suite.template, "Expected dotenv template file to be created")

	variables, contents, dotenvTemplateUsed, err := writeUsingTemplateFile(suite.dotfile)

	suite.NoError(err) //nolint: testifylint

	suite.FileExists(suite.dotfile, "Expected dotenv file to be created")

	suite.Equal(
		[]variable{
			{name: "USE_DATAROBOT_LLM_GATEWAY", value: "", secret: false, changed: false, commented: false},
		},
		variables,
	)
	suite.Equal(
		"USE_DATAROBOT_LLM_GATEWAY=\n",
		contents,
	)
	suite.Equal(suite.template, dotenvTemplateUsed)

	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Verify header format with timestamp and template file reference
	suite.Regexp(`# Edited using .dr dotenv. from .* on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)
	suite.Contains(content, "USE_DATAROBOT_LLM_GATEWAY=\n")

	os.Remove(suite.template)
}

func (suite *TemplateTestSuite) TestReadTemplate() {
	_ = os.WriteFile(suite.template, []byte("USE_DATAROBOT_LLM_GATEWAY=\n"), 0o644)

	suite.FileExists(suite.template, "Expected dotenv template file to be created")

	templateLines, templateFileUsed := readTemplate(suite.dotfile)

	suite.Equal(suite.template, templateFileUsed)
	suite.Equal(
		[]string{"USE_DATAROBOT_LLM_GATEWAY=\n"},
		templateLines,
	)

	os.Remove(suite.template)
}

func (suite *TemplateTestSuite) TestMultipleSavesDoNotDuplicateHeader() {
	_ = os.WriteFile(suite.template, []byte("USE_DATAROBOT_LLM_GATEWAY=\n"), 0o644)

	suite.FileExists(suite.template, "Expected dotenv template file to be created")

	// First save
	_, _, _, err := writeUsingTemplateFile(suite.dotfile)
	suite.Require().NoError(err)

	// Second save (simulating user editing and saving again)
	_, _, _, err = writeUsingTemplateFile(suite.dotfile)
	suite.Require().NoError(err)

	// Read the file and count how many times the header appears
	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Count header comment lines (lines starting with #)
	lines := strings.Split(content, "\n")
	headerCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# Edited using") {
			headerCount++
		}
	}

	// Should only have one header comment
	suite.Equal(1, headerCount, "Expected only one header comment, but found %d", headerCount)

	// Verify the header format with timestamp
	suite.Contains(content, "# Edited using `dr dotenv`")
	suite.Regexp(`# Edited using .dr dotenv. .* on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)

	os.Remove(suite.template)
}
