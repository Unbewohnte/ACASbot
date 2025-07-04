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

type Prompts struct {
	Affiliation    string `json:"affiliation"`
	SentimentShort string `json:"sentiment_short"`
	SentimentLong  string `json:"sentiment_long"`
	Title          string `json:"title"`
}

type OllamaConf struct {
	Model               string  `json:"model"`
	QueryTimeoutSeconds uint    `json:"query_timeout_seconds"`
	Prompts             Prompts `json:"prompts"`
}

type TelegramConf struct {
	ApiToken       string  `json:"api_token"`
	Public         bool    `json:"is_public"`
	AllowedUserIDs []int64 `json:"allowed_user_ids"`
}

type GoogleSheetsConf struct {
	Config          spreadsheet.Config `json:"config"`
	CredentialsFile string             `json:"credentials_file"`
}

type LocalSheetConf struct {
	Filename string `json:"file"`
}

type Sheets struct {
	PushToGoogleSheet bool             `json:"push_to_google_sheet"`
	SaveSheetLocally  bool             `json:"save_sheet_locally"`
	Google            GoogleSheetsConf `json:"google"`
	Local             LocalSheetConf   `json:"local"`
}

type Config struct {
	Telegram       TelegramConf `json:"telegram"`
	Ollama         OllamaConf   `json:"ollama"`
	Sheets         Sheets       `json:"sheets"`
	FullAnalysis   bool         `json:"full_analysis"`
	Object         string       `json:"object"`
	ObjectMetadata string       `json:"object_metadata"`
	MaxContentSize uint         `json:"max_content_size"`
	Debug          bool         `json:"debug"`
}

func Default() *Config {
	return &Config{
		Telegram: TelegramConf{
			ApiToken:       "tg_api_token",
			Public:         true,
			AllowedUserIDs: []int64{},
		},
		Ollama: OllamaConf{
			Model:               "bambucha/saiga-llama3:latest",
			QueryTimeoutSeconds: 300,
			Prompts: Prompts{
				Title:          "Извлеки основной заголовок статьи из следующего текста. Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n{{TEXT}}",
				Affiliation:    "Опиши одним предложением, какая информация в тексте имеет отношение к \"{{OBJECT}}\". Если не имеет, ответь только \"Связи нет\"\n\nТекст:\n{{TEXT}}",
				SentimentShort: "Определи отношение к \"{{OBJECT}}\" в следующем тексте. Варианты: положительный, информационный, отрицательный. Отвечай одним словом. В случае, если нет конкретного отношения, отвечай \"информационный\".\n\nТекст: \n{{TEXT}}",
				SentimentLong:  "Определи отношение к \"{{OBJECT}}\" в тексте. Варианты: положительный, информационный, отрицательный. В случае, если нет конкретного отношения, отвечай \"информационный\". Обоснуй ответ только одним предложением. Формат ответа:\n[отношение одним словом]\nОбоснование: [твое объяснение]\n\nТекст:\n{{TEXT}}",
			},
		},
		Sheets: Sheets{
			SaveSheetLocally:  true,
			PushToGoogleSheet: true,
			Google: GoogleSheetsConf{
				CredentialsFile: "secret.json",
				Config: spreadsheet.NewConfig(
					nil, "spreadsheet_id", "Sheet 1",
				),
			},
			Local: LocalSheetConf{
				Filename: "RESULTS_ACASbot.xlsx",
			},
		},
		Object:         "Жители района, район",
		ObjectMetadata: "",
		MaxContentSize: 3500,
		Debug:          false,
		FullAnalysis:   false,
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
	c.Sheets.Google.Config.CredentialsJSON = nil

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
