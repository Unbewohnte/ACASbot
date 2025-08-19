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
	"Unbewohnte/ACASbot/internal/domain"
	"Unbewohnte/ACASbot/internal/inference"
	"bytes"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tealeg/xlsx/v3"
)

// GenerateFromDatabase создаёт Excel-файл в памяти на основе статей из БД
func GenerateFromDatabase(articles []domain.Article) (*bytes.Buffer, error) {
	file := xlsx.NewFile()
	sheet, err := file.AddSheet("Результаты")
	if err != nil {
		return nil, err
	}

	// Добавляем заголовки
	headerRow := sheet.AddRow()
	headers := []string{
		"Дата добавления", "Дата публикации", "Ресурс", "Заголовок", "URL",
		"Примечание", "Тональность", "Цитирований", "Похожие статьи", "Оригинал?",
	}
	for _, h := range headers {
		cell := headerRow.AddCell()
		cell.Value = h
	}

	// Добавляем данные
	for _, art := range articles {
		row := sheet.AddRow()

		// Дата добавления
		dateAdded := time.Unix(art.CreatedAt, 0)
		cell := row.AddCell()
		cell.SetDate(dateAdded)
		// cell.Value = formatDate(dateAdded)

		// Дата публикации
		if art.PublishedAt > 0 {
			pubDate := time.Unix(art.PublishedAt, 0)
			cell = row.AddCell()
			cell.SetDate(pubDate)
			// cell.Value = formatDate(pubDate)
		} else {
			row.AddCell()
		}

		u, err := url.Parse(art.SourceURL)
		if err != nil {
			u = &url.URL{
				Host: art.SourceURL,
			}
		}
		cell = row.AddCell()
		cell.Value = u.Hostname()

		// Заголовок
		cell = row.AddCell()
		cell.Value = art.Title

		// URL
		cell = row.AddCell()
		cell.Value = art.SourceURL

		// Аффилиация
		cell = row.AddCell()
		cell.Value = art.Affiliation

		// Тональность
		cell = row.AddCell()
		cell.Value = art.Sentiment

		// Цитирований
		cell = row.AddCell()
		cell.SetInt64(art.Citations)

		// Похожие статьи
		cell = row.AddCell()
		cell.Value = strings.Join(art.SimilarURLs, ";")

		// Оригинальность
		cell = row.AddCell()
		if art.Original {
			cell.Value = "Да"
		} else {
			cell.Value = "Нет"
		}
	}

	// Сохраняем в буфер
	buf := new(bytes.Buffer)
	err = file.Write(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// GenerateCustomXLSX создает Excel-файл с настраиваемыми колонками на основе пользовательского конфига
func GenerateCustomXLSX(articles []domain.Article, columns []domain.XLSXColumn, model *inference.Client) (*bytes.Buffer, error) {
	file := xlsx.NewFile()
	sheet, err := file.AddSheet("Результаты")
	if err != nil {
		return nil, err
	}

	// Добавляем заголовки согласно конфигу
	headerRow := sheet.AddRow()
	for _, col := range columns {
		cell := headerRow.AddCell()
		cell.Value = col.Name
	}

	// Обрабатываем статьи
	for _, art := range articles {
		row := sheet.AddRow()

		for _, col := range columns {
			cell := row.AddCell()
			value, err := getColumnValue(art, col, model)
			if err != nil {
				value = "Ошибка: " + err.Error()
			}
			cell.Value = value
		}
	}

	buf := new(bytes.Buffer)
	err = file.Write(buf)
	return buf, err
}

// Универсальный обработчик полей (используется и для LLM, и для прямых полей)
func getField(art domain.Article, fieldName string) (string, error) {
	// Специальные обработки (сохраняем текущую логику)
	switch strings.ToLower(fieldName) {
	case "created_at", "createdat":
		return formatDate(time.Unix(art.CreatedAt, 0)), nil
	case "published_at", "publishedat":
		if art.PublishedAt > 0 {
			return formatDate(time.Unix(art.PublishedAt, 0)), nil
		}
		return "", nil
	case "resource", "hostname":
		u, err := url.Parse(art.SourceURL)
		if err != nil || u == nil {
			return art.SourceURL, nil
		}
		return u.Hostname(), nil
	case "similar_urls", "similarurls":
		return strings.Join(art.SimilarURLs, ";"), nil
	case "original":
		if art.Original {
			return "Да", nil
		} else {
			return "Нет", nil
		}
	}

	// Рефлексия для остальных полей
	r := reflect.ValueOf(art)
	f := reflect.Indirect(r).FieldByNameFunc(func(name string) bool {
		return strings.EqualFold(name, fieldName)
	})
	if !f.IsValid() {
		return "", fmt.Errorf("unknown field: %s", fieldName)
	}

	// Обработка базовых типов
	switch f.Kind() {
	case reflect.String:
		return f.String(), nil
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(f.Int(), 10), nil
	case reflect.Bool:
		return strconv.FormatBool(f.Bool()), nil
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.String {
			strs := make([]string, f.Len())
			for i := 0; i < f.Len(); i++ {
				strs[i] = f.Index(i).String()
			}
			return strings.Join(strs, ";"), nil
		}
	}
	return fmt.Sprintf("%v", f.Interface()), nil
}

// Обработка шаблонов вида {{.FieldName}}
func processTemplate(template string, art domain.Article) (string, error) {
	re := regexp.MustCompile(`{{\.(\w+)}}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Извлекаем имя поля из {{.FieldName}}
		fieldName := match[3 : len(match)-2]

		val, err := getField(art, fieldName)
		if err != nil {
			// Для ошибок возвращаем оригинальный шаблон (или можно panic/log)
			return match
		}
		return val
	}), nil
}

func getColumnValue(art domain.Article, col domain.XLSXColumn, model *inference.Client) (string, error) {
	if col.LLMQuery != "" {
		// Обрабатываем шаблон LLMQuery через универсальный метод
		query, err := processTemplate(col.LLMQuery, art)
		if err != nil {
			return "", err
		}
		response, err := model.Query(query)
		if err != nil {
			return "", fmt.Errorf("LLM query failed: %v", err)
		}
		return response, nil
	}

	if col.Field != "" {
		return getField(art, col.Field)
	}

	return "", fmt.Errorf("не было указано ни field, ни llm_query")
}
