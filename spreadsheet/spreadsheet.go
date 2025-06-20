package spreadsheet

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Config struct {
	CredentialsJSON []byte
	SpreadsheetID   string
	SheetName       string
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
	spreadsheetID string
	sheetName     string
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
		spreadsheetID: conf.SpreadsheetID,
		sheetName:     conf.SheetName,
	}, nil
}

type SheetEntry struct {
	PublicationDate time.Time
	Source          string
	Summary         string
	URL             string
	SentimentType   string
}

// AddAnalysisResult добавляет результат анализа в таблицу
func (gsc *GoogleSheetsClient) AddAnalysisResult(entry *SheetEntry) error {
	// Формируем строку для добавления
	values := []interface{}{
		entry.PublicationDate.Format("2006-01-02"),
		entry.Source,
		entry.Summary,
		entry.URL,
		"", // Пустое примечание
		entry.SentimentType,
	}

	// Создаем запрос на добавление
	row := &sheets.ValueRange{
		Values: [][]interface{}{values},
	}

	// Определяем диапазон для добавления (последняя строка)
	rangeData := gsc.sheetName + "!A:A"
	resp, err := gsc.service.Spreadsheets.Values.Get(gsc.spreadsheetID, rangeData).Do()
	if err != nil {
		return fmt.Errorf("не удалось получить данные: %w", err)
	}

	// Вычисляем следующую пустую строку
	nextRow := len(resp.Values) + 1
	insertRange := fmt.Sprintf("%s!A%d:E%d", gsc.sheetName, nextRow, nextRow)

	// Выполняем запрос
	_, err = gsc.service.Spreadsheets.Values.Append(
		gsc.spreadsheetID,
		insertRange,
		row,
	).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("не удалось добавить данные: %w", err)
	}

	return nil
}

func (gsc *GoogleSheetsClient) AddAnalysisResultWithRetry(entry *SheetEntry, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := gsc.AddAnalysisResult(entry); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
	return lastErr
}

func (gsc *GoogleSheetsClient) BatchAddResults(entries []*SheetEntry) error {
	var vr sheets.ValueRange
	for _, entry := range entries {
		vr.Values = append(vr.Values, []interface{}{
			entry.PublicationDate.Format("2006-01-02"),
			entry.Source,
			entry.Summary,
			entry.URL,
			entry.SentimentType,
		})
	}

	_, err := gsc.service.Spreadsheets.Values.Append(
		gsc.spreadsheetID,
		gsc.sheetName+"!A:E",
		&vr,
	).ValueInputOption("USER_ENTERED").Do()

	return err
}
