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

// Package platform implements `dr platform describe`: what an install says
// about itself that decides whether a Custom Application deploy will work,
// read from public routes only. The report's shape is a contract, written down
// in docs/schemas/platform-describe.schema.json.
package platform

// SchemaVersion is the report's contract version. Changes are additive; a
// breaking change bumps it.
const SchemaVersion = 1

// Status says how much of a section the producer could read.
type Status string

const (
	// StatusOK means the section is complete.
	StatusOK Status = "ok"

	// StatusDegraded means part of the section is missing; Message says which.
	StatusDegraded Status = "degraded"

	// StatusUnavailable means the section could not be read; Message says why.
	StatusUnavailable Status = "unavailable"
)

// Section names. Readers ignore names they do not know.
const (
	SectionInstall               = "install"
	SectionSeats                 = "seats"
	SectionEntitlements          = "entitlements"
	SectionExecutionEnvironments = "executionEnvironments"
	SectionResourceBundles       = "resourceBundles"
)

// Report is the whole answer. It states facts the install reports and leaves
// the verdict ("you can create an app") to the reader.
type Report struct {
	SchemaVersion int                `json:"schemaVersion"`
	GeneratedAt   string             `json:"generatedAt,omitempty"`
	Producer      Producer           `json:"producer"`
	Server        Server             `json:"server"`
	Sections      map[string]Section `json:"sections"`
}

// Producer names the tool that wrote the report.
type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Server identifies the install. A field is empty when its source failed.
type Server struct {
	Release      string `json:"release,omitempty"`
	APIVersion   string `json:"apiVersion,omitempty"`
	CanonicalURL string `json:"canonicalUrl,omitempty"`
}

// Section is one source's result. Data is absent when the source is
// unavailable; Message is present then, and for a degraded section.
type Section struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// Install carries the install-wide switches from the public /config route. A
// key the install does not return stays unset rather than guessed.
type Install struct {
	IsEnterprise             *bool  `json:"isEnterprise,omitempty"`
	DefaultAppResourceBundle string `json:"defaultAppResourceBundle,omitempty"`
}

// Seats is the caller's seat licenses as the install reports them: each license
// key maps to whether the caller has access. A key that is absent is not
// enforced on this install, so an empty map does not mean blocked.
type Seats struct {
	SeatLicenses map[string]bool `json:"seatLicenses"`
}

// ExecutionEnvironments lists every execution environment the caller can read.
type ExecutionEnvironments struct {
	Items []ExecutionEnvironment `json:"items"`
}

// ExecutionEnvironment is one environment. Names are not unique on an install,
// so a reader picks by ID and HasSuccessfulVersion.
type ExecutionEnvironment struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	ProgrammingLanguage  string   `json:"programmingLanguage,omitempty"`
	UseCases             []string `json:"useCases"`
	HasSuccessfulVersion bool     `json:"hasSuccessfulVersion"`
	BuildStatus          string   `json:"buildStatus,omitempty"`
}

// ResourceBundles lists every resource bundle the caller can read.
type ResourceBundles struct {
	Items []ResourceBundle `json:"items"`
}

// ResourceBundle is one bundle. The bundle's use cases say which workloads may
// use it; a Custom Application needs one that lists customApplication.
type ResourceBundle struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	UseCases    []string `json:"useCases"`
	MemoryBytes int64    `json:"memoryBytes"`
	CPUCount    float64  `json:"cpuCount,omitempty"`
}
