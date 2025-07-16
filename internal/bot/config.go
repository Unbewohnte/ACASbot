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

package bot

import (
	"Unbewohnte/ACASbot/internal/db"
	"Unbewohnte/ACASbot/internal/spreadsheet"
	"encoding/json"
	"errors"
	"io"
	"os"
)

var CONFIG_PATH string = ""

type Prompts struct {
	Affiliation string `json:"affiliation"`
	Sentiment   string `json:"sentiment"`
	Title       string `json:"title"`
}

type OllamaConf struct {
	GeneralModel        string  `json:"general_model"`
	QueryTimeoutSeconds uint    `json:"query_timeout_seconds"`
	Prompts             Prompts `json:"prompts"`
	EmbeddingModel      string  `json:"embedding_model"`
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

type Sheets struct {
	PushToGoogleSheet bool             `json:"push_to_google_sheet"`
	Google            GoogleSheetsConf `json:"google"`
}

type DBConf struct {
	File string `json:"file"`
	db   *db.DB
}

type AnalysisConf struct {
	Object              string `json:"object"`
	ObjectMetadata      string `json:"object_metadata"`
	MaxContentSize      uint   `json:"max_content_size"`
	SaveSimilarArticles bool   `json:"save_similar_articles"`
}

type Config struct {
	Telegram TelegramConf `json:"telegram"`
	Ollama   OllamaConf   `json:"ollama"`
	Sheets   Sheets       `json:"sheets"`
	Analysis AnalysisConf `json:"analysis"`
	Debug    bool         `json:"debug"`
	DB       DBConf       `json:"database"`
}

func (c *Config) OpenDB() (*db.DB, error) {
	var err error
	c.DB.db, err = db.NewDB(c.DB.File)
	if err != nil {
		return nil, err
	}

	return c.DB.db, nil
}

func (c *Config) GetDB() *db.DB {
	return c.DB.db
}

func DefaultConfig() *Config {
	return &Config{
		Telegram: TelegramConf{
			ApiToken:       "tg_api_token",
			Public:         true,
			AllowedUserIDs: []int64{},
		},
		Ollama: OllamaConf{
			GeneralModel:        "bambucha/saiga-llama3:latest",
			QueryTimeoutSeconds: 600,
			Prompts: Prompts{
				Title:       "Извлеки основной заголовок статьи из следующего текста. Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n{{TEXT}}",
				Affiliation: "Опиши одним предложением, какая информация в тексте имеет отношение к \"{{OBJECT}}\".\n\nТекст:\n{{TEXT}}",
				Sentiment:   "Определи отношение к \"{{OBJECT}}\" в тексте. Варианты: положительный, информационный, отрицательный. Обоснуй ответ только одним предложением. Формат ответа:\n[отношение одним словом]\nОбоснование: [твое объяснение]\n\nТекст:\n{{TEXT}}",
			},
			EmbeddingModel: "bge-m3:latest",
		},
		Sheets: Sheets{
			PushToGoogleSheet: true,
			Google: GoogleSheetsConf{
				CredentialsFile: "secret.json",
				Config: spreadsheet.NewConfig(
					nil, "spreadsheet_id", "Sheet 1",
				),
			},
		},
		Analysis: AnalysisConf{
			Object:              "Жители, люди",
			ObjectMetadata:      "",
			MaxContentSize:      8000,
			SaveSimilarArticles: true,
		},
		DB: DBConf{
			File: "ACASBOT.sqlite3",
		},
		Debug: false,
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

func ConfigFrom(filepath string) (*Config, error) {
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
