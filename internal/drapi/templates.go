// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package drapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsGlobal    bool   `json:"isGlobal"`
	IsPremium   bool   `json:"isPremium"`

	Readme     string     `json:"readme"`
	Tags       []string   `json:"tags"`
	Repository Repository `json:"repository"`
	MediaURL   string     `json:"mediaURL"`

	CreatedAt time.Time `json:"createdAt"`
	// CreatedBy        string `json:"createdBy"`
	// CreatorFirstName string `json:"creatorFirstName"`
	// CreatorLastName  string `json:"creatorLastName"`
	// CreatorUserhash  string `json:"creatorUserhash"`
	// EditedBy         string `json:"editedBy"`
	// EditorFirstName  string `json:"editorFirstName"`
	// EditorLastName   string `json:"editorLastName"`
	// EditorUserhash   string `json:"editorUserhash"`
	// EditedAt         string `json:"editedAt"`
}

func (t Template) FilterValue() string {
	// return fmt.Sprintf("%s\n%s", t.Name, t.Description)
	return t.Name
}

func (t Template) DefaultDir() string {
	split := strings.Split(t.Repository.URL, "/")
	if len(split) > 0 {
		return split[len(split)-1]
	}

	return ""
}

type Repository struct {
	URL      string `json:"url"`
	Tag      string `json:"tag"`
	IsPublic bool   `json:"isPublic"`
}

type TemplateList struct {
	Templates  []Template `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

func (tl TemplateList) ExcludePremium() TemplateList {
	var filtered []Template

	for _, t := range tl.Templates {
		if !t.IsPremium {
			filtered = append(filtered, t)
		}
	}

	// Updated the template list counts accordingly
	return TemplateList{
		Templates:  filtered,
		Count:      len(filtered),
		TotalCount: len(filtered),
		Next:       tl.Next,
		Previous:   tl.Previous,
	}
}

func (tl TemplateList) SortNewestFirst() TemplateList {
	// Create a copy of the slice to avoid modifying the cached data
	sorted := make([]Template, len(tl.Templates))
	copy(sorted, tl.Templates)

	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	tl.Templates = sorted

	return tl
}

func (tl TemplateList) SortByName() TemplateList {
	sorted := make([]Template, len(tl.Templates))
	copy(sorted, tl.Templates)

	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	tl.Templates = sorted

	return tl
}

func GetTemplates() (*TemplateList, error) {
	datarobotEndpoint, err := config.GetEndpointURL("/api/v2/applicationTemplates/")
	if err != nil {
		return nil, err
	}

	log.Info("Fetching templates from " + datarobotEndpoint)

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint+"?limit=100", nil)
	if err != nil {
		return nil, err
	}

	token, err := config.GetAPIKey()
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("User-Agent", config.GetUserAgentHeader())

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Response status code is " + resp.Status + ".")
	}

	var templateList TemplateList

	err = json.NewDecoder(resp.Body).Decode(&templateList)
	if err != nil {
		return nil, err
	}

	return &templateList, nil
}

func GetPublicTemplatesSorted() (*TemplateList, error) {
	templates, err := GetTemplates()
	if err != nil {
		return nil, err
	}

	result := (*templates).ExcludePremium().SortByName().SortNewestFirst()

	return &result, nil
}

func GetTemplate(id string) (*Template, error) {
	templates, err := GetTemplates()
	if err != nil {
		return nil, err
	}

	for _, template := range templates.Templates {
		if template.ID == id {
			return &template, nil
		}
	}

	return nil, fmt.Errorf("Template with id %s not found.", id)
}
