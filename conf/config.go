/*
   ACASbot - Article Context And Sentiment bot
   Copyright (C) 2025  Unbewohnte (Kasyanov Nikolay Alexeevich)

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package conf

import (
	"Unbewohnte/ACASbot/spreadsheet"
	"encoding/json"
	"errors"
	"io"
	"os"
)

var CONFIG_PATH string = ""

type Config struct {
	ApiToken                  string             `json:"api_token"`
	OrganizationName          string             `json:"organization_name"`
	OllamaModel               string             `json:"ollama_model"`
	OllamaQueryTimeoutSeconds uint               `json:"ollama_query_timeout_seconds"`
	MaxContentSize            uint               `json:"max_content_size"`
	Debug                     bool               `json:"debug"`
	FullAnalysis              bool               `json:"full_analysis"`
	PushToGoogleSheet         bool               `json:"push_to_google_sheet"`
	SheetConfig               spreadsheet.Config `json:"sheet_config"`
	CredentialsFile           string             `json:"credentials_file"`
	Public                    bool               `json:"is_public"`
	AllowedUserIDs            []int64            `json:"allowed_user_ids"`
}

func Default() *Config {
	return &Config{
		ApiToken:          "tg_api_token",
		OrganizationName:  "Жители района, район",
		OllamaModel:       "lakomoor/vikhr-llama-3.2-1b-instruct:1b",
		MaxContentSize:    300,
		Debug:             false,
		FullAnalysis:      false,
		PushToGoogleSheet: true,
		SheetConfig: spreadsheet.NewConfig(
			nil, "spreadsheet_id", "Sheet 1",
		),
		CredentialsFile:           "secret.json",
		Public:                    true,
		AllowedUserIDs:            []int64{},
		OllamaQueryTimeoutSeconds: 300,
	}
}

func (conf *Config) Save(filepath string) error {
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	// Убираем ключи доступа к таблицам
	c := *conf
	c.SheetConfig.CredentialsJSON = nil

	jsonBytes, err := json.MarshalIndent(&c, "", "\t")
	if err != nil {
		return err
	}

	_, err = file.Write(jsonBytes)

	// Запоминаем, куда сохранили
	CONFIG_PATH = filepath

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

	// Запоминаем, откуда взяли
	CONFIG_PATH = filepath

	return &conf, nil
}

// Обновляет конфигурационный файл
func (conf *Config) Update() error {
	if CONFIG_PATH == "" {
		return errors.New("неизвестен путь к конфигурационному файлу")
	}

	return conf.Save(CONFIG_PATH)
}
