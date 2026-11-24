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

package platform

// SchemaVersion is the contract version, written beside the report in the JSON
// output. Once the command is generally available, changes are additive and a
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

// Scope says who a section's answer is true for.
type Scope string

const (
	// ScopePlatform means the answer is the same for any caller.
	ScopePlatform Scope = "platform"

	// ScopeCaller means the answer depends on who asks: their permissions,
	// seats, or organization.
	ScopeCaller Scope = "caller"
)

// Section names. Readers ignore names they do not know.
const (
	SectionInstall               = "install"
	SectionSeats                 = "seats"
	SectionEntitlements          = "entitlements"
	SectionExecutionEnvironments = "executionEnvironments"
	SectionResourceBundles       = "resourceBundles"
	SectionWorkloadAPI           = "workloadApi"
)

// Report is the whole answer. It states facts the install reports and leaves
// the verdict ("you can create an app") to the reader.
type Report struct {
	GeneratedAt string             `json:"generatedAt,omitempty"`
	Producer    Producer           `json:"producer"`
	Server      Server             `json:"server"`
	Sections    map[string]Section `json:"sections"`
}

// Producer names the tool that wrote the report.
type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Server identifies the install. A field is empty when its source failed.
// Release comes from /config. APIVersion comes from /api/v2/version/ and is the
// version the caller sees, which can differ between callers.
type Server struct {
	Release      string `json:"release,omitempty"`
	APIVersion   string `json:"apiVersion,omitempty"`
	CanonicalURL string `json:"canonicalUrl,omitempty"`
}

// Section is one source's result. Data is absent when the source is
// unavailable; Message is present then, and for a degraded section. HTTPStatus
// is the status of the one HTTP error response that made the section
// unavailable, so a reader need not take it from the message. It is absent when
// the failure was not an error status (no response came, or the answer was not
// the JSON the route promises), and for a section that several responses
// decided, such as entitlements.
type Section struct {
	Scope      Scope  `json:"scope"`
	Status     Status `json:"status"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Message    string `json:"message,omitempty"`
	Data       any    `json:"data,omitempty"`
}

// Install carries the install-wide switches from the /config route. A
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
// and any user who can share one may name it like an official one, so a reader
// picks by ID from a list it pins, never by Name. IsPublic says whether the
// install publishes the environment to every user, as it does an official one,
// and stays unset when the install does not say. SourceDockerImageURI is the
// image address of the latest successful version when the install publishes
// one, as a reader needs it for a Dockerfile FROM on an install that pulls only
// its own images. BuildStatus is the status of the latest build, which may
// have failed after an earlier one succeeded.
type ExecutionEnvironment struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	ProgrammingLanguage  string   `json:"programmingLanguage,omitempty"`
	UseCases             []string `json:"useCases"`
	HasSuccessfulVersion bool     `json:"hasSuccessfulVersion"`
	BuildStatus          string   `json:"buildStatus,omitempty"`
	SourceDockerImageURI string   `json:"sourceDockerImageUri,omitempty"`
	IsPublic             *bool    `json:"isPublic,omitempty"`
}

// WorkloadAPI is what the Workload API's list route answers the caller with. A
// refusal is not data: the section is unavailable and its message carries the
// body of the service that refused, which names the cause. TotalCount stays
// unset when the route does not return it.
type WorkloadAPI struct {
	TotalCount *int `json:"totalCount,omitempty"`
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
	CPUCount    *float64 `json:"cpuCount,omitempty"`
}
