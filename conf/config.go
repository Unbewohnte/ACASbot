package conf

import (
	"Unbewohnte/ACATbot/spreadsheet"
	"encoding/json"
	"io"
	"os"
)

type Config struct {
	ApiToken          string             `json:"api_token"`
	OrganizationName  string             `json:"organization_name"`
	OllamaModel       string             `json:"ollama_model"`
	MaxContentSize    uint               `json:"max_content_size"`
	Debug             bool               `json:"debug"`
	FullAnalysis      bool               `json:"full_analysis"`
	PushToGoogleSheet bool               `json:"push_to_google_sheet"`
	SheetConfig       spreadsheet.Config `json:"sheet_config"`
	Public            bool               `json:"is_public"`
	AllowedUserIDs    []int64            `json:"allowed_user_ids"`
}

func NewConfig(
	apiToken string,
	organizationName string,
	ollamaModel string,
	maxContentSize uint,
	debug bool,
	fullAnalysis bool,
	pushToSheet bool,
	sheetsConf spreadsheet.Config,
	public bool,
	allowedUserIDs []int64,
) *Config {
	return &Config{
		ApiToken:          apiToken,
		OrganizationName:  organizationName,
		OllamaModel:       ollamaModel,
		MaxContentSize:    maxContentSize,
		Debug:             debug,
		FullAnalysis:      fullAnalysis,
		PushToGoogleSheet: pushToSheet,
		SheetConfig:       sheetsConf,
		Public:            public,
		AllowedUserIDs:    allowedUserIDs,
	}
}

func (conf *Config) Save(filepath string) error {
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonBytes, err := json.MarshalIndent(conf, "", "\t")
	if err != nil {
		return err
	}

	_, err = file.Write(jsonBytes)

	return err
}

func From(filepath string) (*Config, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var conf Config
	err = json.Unmarshal(contents, &conf)
	if err != nil {
		return nil, err
	}

	return &conf, nil
}
