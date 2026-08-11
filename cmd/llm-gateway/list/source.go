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

package list

import (
	"fmt"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/spf13/pflag"
)

// Source selects which LLM sources the list command queries. Its gateway and
// deployed values are the strings the SOURCE column and the JSON "source" field
// already carry, so a filter can be copied straight out of a listing.
type Source string

const (
	SourceAll      Source = "all"
	SourceGateway  Source = drapi.LLMKindGateway
	SourceDeployed Source = drapi.LLMKindDeployed
)

var _ pflag.Value = (*Source)(nil)

func (s *Source) String() string {
	if s == nil {
		return ""
	}

	return string(*s)
}

func (s *Source) Set(v string) error {
	switch Source(v) {
	case SourceAll, SourceGateway, SourceDeployed:
		*s = Source(v)

		return nil
	}

	return fmt.Errorf("invalid source %q: use %s, %s, or %s", v, SourceAll, SourceGateway, SourceDeployed)
}

func (s *Source) Type() string {
	return "source"
}

// fetchLLMs skips the request the unselected source would have made.
//
// A single-source request returns its error rather than degrading to an empty
// list. The union path can fall back on the other source; here there is no
// remainder to show, and "no models" for an unreachable catalog reads as an
// empty instance.
func fetchLLMs(source Source) (*drapi.LLMList, error) {
	if source == SourceGateway {
		gateway, err := drapi.GetLLMs()
		if err != nil {
			return nil, err
		}

		// GetLLMs leaves Count and TotalCount as the last decoded page reported
		// them, which is not the filtered row count. Restate them over the rows
		// actually returned so all three branches agree on what the fields mean.
		gateway.Count, gateway.TotalCount = len(gateway.LLMs), len(gateway.LLMs)

		return gateway, nil
	}

	if source == SourceDeployed {
		deployed, err := drapi.GetDeployedLLMs()
		if err != nil {
			return nil, err
		}

		return &drapi.LLMList{LLMs: deployed, Count: len(deployed), TotalCount: len(deployed)}, nil
	}

	// SourceAll, and a zero value that never went through Set.
	return drapi.GetLLMsAndDeployed()
}
