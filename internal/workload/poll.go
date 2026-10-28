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

package workload

// transientBudget is the "forgive a blip, not an outage" policy the long waits
// in this package share: count consecutive transient failures, reset on any
// success, give up once the run exceeds maxTransientPollErrors.
//
// It exists because the policy was being re-derived at each call site, with the
// comparisons already drifting apart, and because the waits that grew long
// enough to need it are exactly the ones where a silent retry is indistinct
// from a hang.
type transientBudget struct {
	spent int
}

// forgive reports whether err is worth looking past. A caller that gets false
// should return the error.
func (b *transientBudget) forgive(err error) bool {
	if !isTransientPollError(err) || b.spent >= maxTransientPollErrors {
		return false
	}

	b.spent++

	return true
}

// reset forgets the run so far, and is called on any poll that worked.
func (b *transientBudget) reset() {
	b.spent = 0
}
