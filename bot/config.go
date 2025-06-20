package bot

import "Unbewohnte/ACATbot/spreadsheet"

type Config struct {
	ApiToken          string
	OrganizationName  string
	OllamaModel       string
	MaxContentSize    uint
	Debug             bool
	FullAnalysis      bool
	PushToGoogleSheet bool
	SheetConfig       spreadsheet.Config
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
) Config {
	return Config{
		ApiToken:          apiToken,
		OrganizationName:  organizationName,
		OllamaModel:       ollamaModel,
		MaxContentSize:    maxContentSize,
		Debug:             debug,
		FullAnalysis:      fullAnalysis,
		PushToGoogleSheet: pushToSheet,
		SheetConfig:       sheetsConf,
	}
}
