// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package shared

import (
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/copier"
)

type ListItem struct {
	current   bool
	checked   bool
	component copier.Answers
}

func (i ListItem) Title() string {
	return fmt.Sprintf("%s (%s)",
		i.component.ComponentDetails.Name,
		i.component.FileName,
	)
}

// TODO: Decide if we return something for description - don't think needed - it's really just us adhering to interface
func (i ListItem) Description() string { return "" }

func (i ListItem) FilterValue() string {
	return strings.ToLower(i.component.FileName)
}
