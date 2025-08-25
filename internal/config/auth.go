package config

import (
	"fmt"
	"net/url"

	"github.com/spf13/viper"
)

func loadBaseURLFromURL(longURL string) (string, error) {
	// Takes a URL like: https://app.datarobot.com/api/v2 and just
	// returns https://app.datarobot.com (no trailing slash)
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return "", err
	}

	base := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	return base, nil
}

func GetBaseURL() (string, error) {
	urlContent := viper.GetString(DataRobotURL)

	if urlContent == "" {
		return "", nil
	}

	baseURL, err := loadBaseURLFromURL(urlContent)
	if err != nil {
		return "", err
	}

	return baseURL, nil
}

func SaveURLToConfig(newURL string) error {
	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	if newURL == "" {
		viper.Set(DataRobotAPIKey, "")
	}

	baseURL, err := loadBaseURLFromURL(newURL)
	if err != nil {
		return err
	}

	datarobotHost, err := url.JoinPath(baseURL, "/api/v2")
	if err != nil {
		return err
	}

	viper.Set(DataRobotURL, datarobotHost)

	_ = viper.WriteConfig()

	return nil
}

func LoadBaseURLFromURL(longURL string) (string, error) {
	// Takes a URL like: https://app.datarobot.com/api/v2 and just
	// returns https://app.datarobot.com (no trailing slash)
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return "", err
	}

	base := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	return base, nil
}
