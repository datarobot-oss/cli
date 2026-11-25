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

// Package doctor holds the checks that read a `dr info` report and say whether
// the caller can use the install. Each check is a pure function of the report,
// so a run reads the install once, through platform.Describe, and the checks
// also run offline on a saved report.
//
// Check IDs sit under the "platform." namespace. They are stable and their
// order is public, like the IDs of the checks in internal/workload/doctor.
package doctor
