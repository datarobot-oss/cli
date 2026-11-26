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

package llm

import (
	"fmt"

	"github.com/datarobot/cli/internal/drapi"
)

// maxPages bounds one paged read, as internal/platform does: a server that
// never stops handing out next links ends the command with an error rather
// than running it forever.
const maxPages = 100

// pager follows a list's next links and refuses the ones that would not make
// progress: a page seen before, or an empty page that still points onwards.
type pager struct {
	seen  map[string]bool
	pages int
}

func newPager(first string) *pager {
	return &pager{seen: map[string]bool{first: true}}
}

// next returns the URL of the page after one with count items and the given
// next link, "" when the list is done, or an error when following it would
// loop, leave the configured host, or exceed maxPages.
func (p *pager) next(next string, count int) (string, error) {
	if next == "" {
		return "", nil
	}

	p.pages++

	if p.pages >= maxPages {
		return "", fmt.Errorf("the list did not finish within %d pages; narrow it with filters", maxPages)
	}

	if count == 0 {
		return "", fmt.Errorf("the server returned an empty page that still links to another (%s)", next)
	}

	if p.seen[next] {
		return "", fmt.Errorf("the server's next link points back to a page already read (%s)", next)
	}

	if err := drapi.AssertNextOnSameHost(next); err != nil {
		return "", err
	}

	p.seen[next] = true

	return next, nil
}
