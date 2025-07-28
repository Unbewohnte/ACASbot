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

package spreadsheet

import (
	"Unbewohnte/ACASbot/internal/article"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Config struct {
	CredentialsJSON []byte `json:"credentails"`
	SpreadsheetID   string `json:"spreadsheet_id"`
	SheetName       string `json:"sheet_name"`
}

func NewConfig(credentialsJSON []byte,
	spreadsheetID string,
	sheetName string,
) Config {
	return Config{
		CredentialsJSON: credentialsJSON,
		SpreadsheetID:   spreadsheetID,
		SheetName:       sheetName,
	}
}

type GoogleSheetsClient struct {
	service       *sheets.Service
	SpreadsheetID string
	SheetName     string
}

func NewGoogleSheetsClient(ctx context.Context, conf Config) (*GoogleSheetsClient, error) {
	// Аутентификация с использованием сервисного аккаунта
	config, err := google.JWTConfigFromJSON(
		conf.CredentialsJSON,
		sheets.SpreadsheetsScope,
	)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать JWT конфиг: %w", err)
	}

	client := config.Client(ctx)

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("не удалось создать Google Sheets сервис: %w", err)
	}

	return &GoogleSheetsClient{
		service:       srv,
		SpreadsheetID: conf.SpreadsheetID,
		SheetName:     conf.SheetName,
	}, nil
}

func formatDate(date time.Time) string {
	return fmt.Sprintf("%d.%d.%d", date.Day(), date.Month(), date.Year())
}

// AddAnalysisResult добавляет результат анализа в таблицу
func (gsc *GoogleSheetsClient) AddAnalysisResult(art *article.Article) error {
	// Формируем строку для добавления
	u, err := url.Parse(art.SourceURL)
	if err != nil {
		u = &url.URL{
			RawPath: art.SourceURL,
		}
	}

	values := []interface{}{
		formatDate(time.Unix(art.PublishedAt, 0)),
		u.Hostname(),
		art.Title,
		art.SourceURL,
		art.Affiliation,
		art.Sentiment,
		art.Citations,
		strings.Join(art.SimilarURLs, ";"),
	}

	// Создаем запрос на добавление
	row := &sheets.ValueRange{
		Values: [][]interface{}{values},
	}

	// Определяем диапазон для добавления (последняя строка)
	rangeData := gsc.SheetName + "!A:A"
	resp, err := gsc.service.Spreadsheets.Values.Get(gsc.SpreadsheetID, rangeData).Do()
	if err != nil {
		return fmt.Errorf("не удалось получить данные: %w", err)
	}

	// Вычисляем следующую пустую строку
	nextRow := len(resp.Values) + 1
	insertRange := fmt.Sprintf("%s!A%d:E%d", gsc.SheetName, nextRow, nextRow)

	// Выполняем запрос
	_, err = gsc.service.Spreadsheets.Values.Append(
		gsc.SpreadsheetID,
		insertRange,
		row,
	).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("не удалось добавить данные: %w", err)
	}

	return nil
}

func (gsc *GoogleSheetsClient) AddAnalysisResultWithRetry(art *article.Article, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := gsc.AddAnalysisResult(art); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
	return lastErr
}

func (gsc *GoogleSheetsClient) BatchAddResults(articles []*article.Article) error {
	var vr sheets.ValueRange
	for _, art := range articles {
		u, err := url.Parse(art.SourceURL)
		if err != nil {
			u = &url.URL{
				RawPath: art.SourceURL,
			}
		}

		vr.Values = append(vr.Values, []interface{}{
			formatDate(time.Unix(art.PublishedAt, 0)),
			u.Hostname(),
			art.Title,
			art.SourceURL,
			art.Affiliation,
			art.Sentiment,
			art.Citations,
			strings.Join(art.SimilarURLs, ";"),
		})
	}

	_, err := gsc.service.Spreadsheets.Values.Append(
		gsc.SpreadsheetID,
		gsc.SheetName+"!A:E",
		&vr,
	).ValueInputOption("USER_ENTERED").Do()

	return err
}
