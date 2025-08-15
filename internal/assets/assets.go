// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package assets

import (
	"embed"
	"io"
)

//go:embed templates/*.html
var embedded embed.FS

func Write(w io.Writer, name string) error {
	content, err := embedded.ReadFile(name)
	if err != nil {
		return err
	}

	_, err = w.Write(content)
	return err
}
