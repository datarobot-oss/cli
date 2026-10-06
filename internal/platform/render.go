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

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/datarobot/cli/internal/outputformat"
)

// customApplication is the use case that marks an environment or a resource
// bundle as valid for a Custom Application.
const customApplication = "customApplication"

// sectionOrder is the order the text view lists sections in.
var sectionOrder = []string{
	SectionInstall,
	SectionSeats,
	SectionEntitlements,
	SectionExecutionEnvironments,
	SectionResourceBundles,
}

// RenderTo writes the report to w. JSON is the report itself, the contract in
// docs/schemas; text is a summary for a person.
func RenderTo(w io.Writer, format outputformat.OutputFormat, report Report) error {
	if format == outputformat.OutputFormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")

		return enc.Encode(report)
	}

	return renderText(w, report)
}

func renderText(w io.Writer, report Report) error {
	var err error

	// Writes to w are checked once, on the last one: a writer that fails stays failed.
	say := func(format string, args ...any) {
		_, err = fmt.Fprintf(w, format+"\n", args...)
	}

	say("Install   %s", orUnknown(report.Server.CanonicalURL))
	say("Release   %s (API %s)", orUnknown(report.Server.Release), orUnknown(report.Server.APIVersion))

	for _, name := range sectionOrder {
		section, ok := report.Sections[name]
		if !ok {
			continue
		}

		say("")
		say("%s: %s", name, section.Status)

		if section.Message != "" {
			say("  %s", section.Message)
		}

		renderData(say, section.Data)
	}

	return err
}

func renderData(say func(string, ...any), data any) {
	switch d := data.(type) {
	case Install:
		if d.IsEnterprise != nil {
			say("  enterprise install: %s", yesNo(*d.IsEnterprise))
		}

		if d.DefaultAppResourceBundle != "" {
			say("  default app resource bundle: %s", d.DefaultAppResourceBundle)
		}
	case Seats:
		if len(d.SeatLicenses) == 0 {
			say("  no seat licenses enforced")
		}

		for _, name := range sortedKeys(d.SeatLicenses) {
			say("  %s: %s", name, yesNo(d.SeatLicenses[name]))
		}
	case map[string]bool:
		for _, name := range sortedKeys(d) {
			say("  %s: %s", name, onOff(d[name]))
		}
	case ExecutionEnvironments:
		renderEnvironments(say, d)
	case ResourceBundles:
		renderBundles(say, d)
	}
}

func renderEnvironments(say func(string, ...any), envs ExecutionEnvironments) {
	var usable []ExecutionEnvironment

	for _, env := range envs.Items {
		if slices.Contains(env.UseCases, customApplication) {
			usable = append(usable, env)
		}
	}

	say("  %d of %d can run custom applications", len(usable), len(envs.Items))

	for _, env := range usable {
		built := "not built"
		if env.HasSuccessfulVersion {
			built = "built"
		}

		say("  %s  %s  %s  %s", env.ID, orUnknown(env.ProgrammingLanguage), built, env.Name)
	}
}

func renderBundles(say func(string, ...any), bundles ResourceBundles) {
	var usable []ResourceBundle

	for _, bundle := range bundles.Items {
		if slices.Contains(bundle.UseCases, customApplication) {
			usable = append(usable, bundle)
		}
	}

	say("  %d of %d can run custom applications", len(usable), len(bundles.Items))

	for _, bundle := range usable {
		say("  %s  %s  %.4g GiB", bundle.ID, bundle.Name, float64(bundle.MemoryBytes)/(1<<30))
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}

	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}

func onOff(b bool) string {
	if b {
		return "on"
	}

	return "off"
}
