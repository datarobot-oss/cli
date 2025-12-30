// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package reader

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/muesli/cancelreader"
)

func ReadString() (string, error) {
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return "", err
	}

	cancelChan := make(chan os.Signal, 1)
	defer close(cancelChan)

	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(cancelChan)

	go func() {
		<-cancelChan
		cr.Cancel()
	}()

	reader := bufio.NewReader(cr)

	str, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println()
	}

	return str, err
}
