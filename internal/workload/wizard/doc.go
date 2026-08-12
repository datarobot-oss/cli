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

// Package wizard is the setup flow behind `dr workload config`: the handful
// of questions that turn a directory into a committed .datarobot.yaml, and
// the headless path that answers the same questions from flags.
//
// Setup is a one-time act. Run refuses to touch an existing manifest and
// points at it instead, because after the first write the file is the
// interface and editing twelve lines of YAML beats re-answering eight
// questions. Deleting the file re-arms the wizard.
//
// The flow lives here rather than in the command so `dr workload up` can
// embed the identical wizard when it finds no manifest, instead of growing a
// second, subtly different one.
//
// Defaults come from wherever the truth already lives: Detect reads the
// project (a Dockerfile, its EXPOSE), and binding to an existing workload
// reads that workload's own spec. Answers carries what the flags already
// said, and every answered question skips its screen, so the same code path
// serves a laptop and CI. Without a terminal nothing prompts: a missing
// required answer is an error that names the flag.
//
// Non-scope: the manifest format itself (that is the manifest package, which
// renders, validates and writes the file), the .env import and its
// credentials (a separate command flag and a separate ticket), and the NIM
// track.
package wizard
