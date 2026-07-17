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

// image_output.go centralises the human/JSON output rendering used by
// the `dr pipeline image` verbs so each verb file stays focused on
// flag wiring.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

// imageVersionJSON is the DTO for a single ImageVersion in JSON output.
type imageVersionJSON struct {
	Version     int      `json:"version"`
	Pip         []string `json:"pip,omitempty"`
	Conda       any      `json:"conda,omitempty"`
	BaseImage   *string  `json:"base_image,omitempty"`
	Nvidia      bool     `json:"nvidia,omitempty"`
	Status      string   `json:"status"`
	ErrorDetail *string  `json:"error_detail,omitempty"`
	ImageURI    *string  `json:"image_uri,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// imageJSON is the CLI-facing DTO for `--output-format json` of an Image.
type imageJSON struct {
	ImageID       string             `json:"image_id"`
	Name          string             `json:"name"`
	Description   *string            `json:"description,omitempty"`
	LatestVersion int                `json:"latest_version"`
	Versions      []imageVersionJSON `json:"versions"`
	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
}

// imageSummaryJSON is the CLI-facing DTO for `--output-format json` of an ImageSummary.
type imageSummaryJSON struct {
	ImageID       string  `json:"image_id"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	LatestVersion int     `json:"latest_version"`
	LatestStatus  string  `json:"latest_status"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func toImageJSON(img Image) imageJSON {
	versions := make([]imageVersionJSON, len(img.Versions))

	for i, v := range img.Versions {
		ver := imageVersionJSON{
			Version:     v.Version,
			Pip:         v.Definition.Pip,
			BaseImage:   v.Definition.BaseImage,
			Nvidia:      v.Definition.Nvidia,
			Status:      string(v.Status),
			ErrorDetail: v.ErrorDetail,
			ImageURI:    v.ImageURI,
			CreatedAt:   v.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:   v.UpdatedAt.UTC().Format(time.RFC3339),
		}

		if v.Definition.Conda != nil && (len(v.Definition.Conda.Deps) > 0 || len(v.Definition.Conda.Channels) > 0) {
			ver.Conda = v.Definition.Conda
		}

		versions[i] = ver
	}

	return imageJSON{
		ImageID:       img.ImageID,
		Name:          img.Name,
		Description:   img.Description,
		LatestVersion: img.LatestVersion,
		Versions:      versions,
		CreatedAt:     img.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     img.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toImageSummaryJSON(img ImageSummary) imageSummaryJSON {
	return imageSummaryJSON{
		ImageID:       img.ImageID,
		Name:          img.Name,
		Description:   img.Description,
		LatestVersion: img.LatestVersion,
		LatestStatus:  string(img.LatestStatus),
		CreatedAt:     img.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     img.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RenderImage routes a single image to JSON or human output.
func RenderImage(format outputformat.OutputFormat, img Image) error {
	if format == outputformat.OutputFormatJSON {
		return printImageJSON(img)
	}

	printImageHuman(img)

	return nil
}

// RenderImages routes a list of images to JSON or human output.
func RenderImages(format outputformat.OutputFormat, items []ImageSummary) error {
	if format == outputformat.OutputFormatJSON {
		return printImageListJSON(items)
	}

	printImageListHuman(items)

	return nil
}

// printImageJSON marshals an image record as indented JSON through the DTO.
func printImageJSON(img Image) error {
	data, err := json.MarshalIndent(toImageJSON(img), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// printImageHuman renders the key facts about a single image record,
// including its full version history.
func printImageHuman(img Image) {
	desc := emptyValuePlaceholder
	if img.Description != nil && *img.Description != "" {
		desc = *img.Description
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Image ID:\t%s\n", img.ImageID)
	fmt.Fprintf(w, "Name:\t%s\n", img.Name)
	fmt.Fprintf(w, "Description:\t%s\n", desc)
	fmt.Fprintf(w, "Latest version:\tv%s\n", strconv.Itoa(img.LatestVersion))
	fmt.Fprintf(w, "Created:\t%s\n", img.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", img.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()

	printImageVersionsHuman(img.Versions)
}

func printImageVersionsHuman(versions []ImageVersion) {
	if len(versions) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Versions:"))

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"VERSION", "STATUS", "PIP", "CONDA", "BASE IMAGE", "UPDATED"}

	updatedCol := slices.Index(headers, "UPDATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == updatedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, ver := range versions {
		baseImageStr := emptyValuePlaceholder
		if ver.Definition.BaseImage != nil && *ver.Definition.BaseImage != "" {
			baseImageStr = *ver.Definition.BaseImage
		}

		condaStr := emptyValuePlaceholder
		if ver.Definition.Conda != nil && (len(ver.Definition.Conda.Deps) > 0 || len(ver.Definition.Conda.Channels) > 0) {
			condaStr = formatCondaCell(ver.Definition.Conda)
		}

		t.Row(
			fmt.Sprintf("v%d", ver.Version),
			string(ver.Status),
			joinPackages(ver.Definition.Pip),
			condaStr,
			baseImageStr,
			ver.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// printImageListJSON marshals a list of images as indented JSON through the DTO.
func printImageListJSON(items []ImageSummary) error {
	view := make([]imageSummaryJSON, len(items))

	for i, img := range items {
		view[i] = toImageSummaryJSON(img)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// printImageListHuman renders a lipgloss table summary of images.
func printImageListHuman(items []ImageSummary) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No images found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"IMAGE ID", "NAME", "LATEST", "STATUS", "UPDATED"}

	updatedCol := slices.Index(headers, "UPDATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == updatedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, img := range items {
		t.Row(
			img.ImageID,
			img.Name,
			fmt.Sprintf("v%d", img.LatestVersion),
			string(img.LatestStatus),
			img.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// formatCondaCell renders a CondaValue for the human-readable versions table.
// When channels are present they are shown as a bracketed prefix so the user
// can see the full structured spec, not just the dependency list.
//
//	plain list form:        "scipy, numpy"
//	CondaSpec (no deps):    "[conda-forge]"
//	CondaSpec (with deps):  "[conda-forge] scipy, numpy"
func formatCondaCell(conda *CondaValue) string {
	chanPart := ""
	if len(conda.Channels) > 0 {
		chanPart = "[" + strings.Join(conda.Channels, ",") + "]"
	}

	depPart := joinPackages(conda.Deps)

	if chanPart == "" {
		return depPart
	}

	if depPart == "" || depPart == emptyValuePlaceholder {
		return chanPart
	}

	return chanPart + " " + depPart
}

// joinPackages collapses a package slice into a single comma-separated
// string for tabular display, truncating at a reasonable width so the
// table stays readable in a typical terminal.
func joinPackages(packages []string) string {
	const maxLen = 60

	if len(packages) == 0 {
		return emptyValuePlaceholder
	}

	joined := strings.Join(packages, ",")
	if len(joined) <= maxLen {
		return joined
	}

	return joined[:maxLen-3] + "..."
}

// NormalizePackageList normalises a raw StringSliceVar into a trimmed,
// comma-split list. Unlike NormalizePackages it returns nil (not an error)
// when the input is empty, for use in commands where pip is optional.
func NormalizePackageList(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}

	out := make([]string, 0, len(raw))

	for _, entry := range raw {
		for _, item := range strings.Split(entry, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}

	return out
}

// BuildCondaValue converts raw --conda and --conda-channel flag slices
// into a *CondaValue suitable for image requests. Returns nil when both
// slices are empty so the caller can omit the field.
func BuildCondaValue(rawDeps, rawChannels []string) *CondaValue {
	deps := NormalizePackageList(rawDeps)
	channels := NormalizePackageList(rawChannels)

	if len(deps) == 0 && len(channels) == 0 {
		return nil
	}

	return &CondaValue{
		Deps:     deps,
		Channels: channels,
	}
}
