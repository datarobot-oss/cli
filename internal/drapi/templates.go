// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package drapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

	// CreatedBy        string `json:"createdBy"`
	// CreatorFirstName string `json:"creatorFirstName"`
	// CreatorLastName  string `json:"creatorLastName"`
	// CreatorUserhash  string `json:"creatorUserhash"`
	// CreatedAt        string `json:"createdAt"`
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

func GetTemplates() (*TemplateList, error) {
	bearer := "Bearer " + config.GetAPIKey()

	// datarobotHost := "https://staging.datarobot.com"
	// datarobotHost := "https://app.datarobot.com"
	datarobotHost := config.GetBaseURL()

	datarobotEndpoint := datarobotHost + "/api/v2/applicationTemplates/?limit=100"
	log.Info("Fetching templates from " + datarobotEndpoint)

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", bearer)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Response status code is " + resp.Status)
	}

	var templateList TemplateList

	err = json.NewDecoder(resp.Body).Decode(&templateList)
	if err != nil {
		return nil, err
	}

	return &templateList, nil
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

	return nil, fmt.Errorf("template with id %s not found", id)
}
