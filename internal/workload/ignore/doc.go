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

// Package ignore decides which files the sync engine excludes. The
// effective set is the union of hardcoded system excludes (the state
// directory, .git)
// and patterns from <project-root>/.drignore in gitignore syntax.
//
// A project set up before the rename has a .wapiignore instead. That name is
// still read when no .drignore is present, and Matcher.Notice says so, but it
// is deprecated and nothing writes it any more.
//
// It also owns the filenames themselves, for the package that seeds a starter
// file rather than reading one: FileName is what to write, and Locate answers
// whether a project already has one under either name. Keeping both sides on
// this package's answer is what stops a project being given a name that a
// later sync does not look for.
package ignore
