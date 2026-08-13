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

// Package up is the deploy behind `dr workload up`: read the manifest and the
// live workload, work out what differs, and later apply only that.
//
// A deploy is a function of the repository checkout and nothing else. This
// package never reads .env and never expands shell variables into a deploy.
// Anything a container needs at runtime is in the manifest, as a literal or
// as a credential reference; anything kept only in .env would not reach the
// container, which is why importing it is the setup wizard's job and happens
// once, not on every deploy.
//
// Drift is measured against the running object, not against a record of the
// last deploy, and it is one-directional. Whatever the file says, `up` makes
// true; whatever the file leaves out, `up` leaves alone. So a workload can
// carry a GPU bundle, a sidecar and an autoscaling policy this CLI has no
// vocabulary for and none of it is drift, while an edit made in the UI to a
// field the file does name is reported and restored. Deleting a line stops
// managing a field rather than reverting it. A stored hash of the last
// applied spec could satisfy none of that: it cannot see the UI edit, and it
// is absent in a fresh CI clone.
//
// Reading is split from deciding. Load turns a directory into a validated,
// compiled manifest; Look turns a workload id into the live spec, the live
// runtime and a State. Neither judges what it finds: a manifest that is
// missing and a workload that has been deleted are both reported as facts,
// because whether either is fatal depends on what the caller is permitted to
// do about it, and only the command knows that.
//
// The comparison is done on the compiled payload rather than the file, so the
// dr-credential shorthand is already expanded and a credential reference does
// not read as a permanent difference against the object form the platform
// stores. It is done on the server's own documents rather than typed structs,
// because a typed round trip silently drops every field this release has not
// heard of, and half the point is that those survive.
//
// Non-scope: no terminal output and no cobra. Rendering a plan and running
// the phases belong to the command; this package hands back values.
package up
