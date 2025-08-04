package drapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsGlobal    bool   `json:"isGlobal"`
	IsPremium   bool   `json:"isPremium"`
}

func (t Template) FilterValue() string {
	// return fmt.Sprintf("%s\n%s", t.Name, t.Description)
	return t.Name
}

type TemplateList struct {
	Templates  []Template `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

func GetTemplates() (*TemplateList, error) {
	key, err := auth.GetAPIKey()
	if err != nil {
		return nil, err
	}

	bearer := "Bearer " + key

	// datarobotHost := "https://staging.datarobot.com/api/v2"
	// datarobotHost := "https://app.datarobot.com/api/v2"
	datarobotHost, err := auth.GetURL(false)
	if err != nil {
		return nil, err
	}

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
