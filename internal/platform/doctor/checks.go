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

package doctor

import (
	"context"
	"sync"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/platform"
)

// DescribeFunc reads the install.
type DescribeFunc func(ctx context.Context) (*platform.Report, error)

// snapshot shares one read of the install between the checks.
type snapshot struct {
	once     sync.Once
	describe DescribeFunc
	report   *platform.Report
	err      error
}

// get reads the install on the first call only.
func (s *snapshot) get(ctx context.Context) (*platform.Report, error) {
	s.once.Do(func() {
		s.report, s.err = s.describe(ctx)
	})

	return s.report, s.err
}

// Checks returns the platform checks in run order.
func Checks(describe DescribeFunc) []core.Check {
	shared := &snapshot{describe: describe}

	return []core.Check{
		&installRead{snapshot: shared},
		&workloadDeploy{snapshot: shared},
	}
}
