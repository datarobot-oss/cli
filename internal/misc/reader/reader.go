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

package reader

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/muesli/cancelreader"
)

func ReadString() (string, error) {
	if runtime.GOOS == "windows" {
		return ReadStringPlain()
	}

	return ReadStringCancellable()
}

func ReadStringPlain() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	str, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println()
	}

	return str, err
}

func ReadStringCancellable() (string, error) {
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
