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
// A deploy comes in two shapes, decided by whether the manifest names a
// published image or asks the platform to build one. The first is a single
// create. The second has to make an artifact, push the working tree to it and
// wait for an image before there is anything for a workload to run, and it
// does them in that order so a run that dies partway leaves something the
// next one picks up rather than a workload pointing at an image that does not
// exist. Which artifact a checkout pushes to is local state, kept beside the
// code rather than in the committed manifest, because it is a property of the
// clone and not of the project.
//
// A workload that already exists is rolled rather than created again: a new
// version is minted, the platform swaps onto it, and the endpoint survives.
// The version a file names by id is promoted as it stands, and one the
// platform builds is filled first, by the same three acts as a first deploy.
// The order there is what keeps it safe to run against something in use: the
// project is pointed at the new version before the code is pushed, so an
// upload can never rewrite what is currently serving. A run that dies between
// minting a version and promoting it leaves a draft nothing points at, and
// the next run continues with it rather than adding another to the pile.
//
// Locking is one-way, and it is the rule both deploy shapes have to obey the
// same way: a locked artifact can take neither new code nor a new image, so
// the next version of a locked thing is a new artifact rather than a change to
// it. It joins the lineage the locked one belongs to, the checkout's link
// moves onto it, and the code the locked one was running is carried across
// when the working tree has nothing to upload. Whether the successor is itself
// locked is the one part that differs: a roll onto a locked version has to
// lock it, because the platform refuses to put a draft where a locked artifact
// was, while a first deploy leaves it a draft unless the run asked for a lock.
// A deploy that refused instead would be a one-way door, since a workload that
// must stay up has to be locked and a locked one could then never be changed.
//
// Non-scope: no terminal output and no cobra. Rendering a plan and running
// the phases belong to the command; this package hands back values.
package up
