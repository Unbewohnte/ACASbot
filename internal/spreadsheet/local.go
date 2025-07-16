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
	"bytes"
	"strings"
	"time"

	"github.com/tealeg/xlsx/v3"
)

// GenerateFromDatabase создаёт Excel-файл в памяти на основе статей из БД
func GenerateFromDatabase(articles []article.Article) (*bytes.Buffer, error) {
	file := xlsx.NewFile()
	sheet, err := file.AddSheet("Результаты")
	if err != nil {
		return nil, err
	}

	// Добавляем заголовки
	headerRow := sheet.AddRow()
	headers := []string{
		"Дата добавления", "Дата публикации", "Заголовок", "URL",
		"Аффилиация", "Тональность", "Цитирований", "Похожие статьи",
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

		// Дата публикации
		if art.PublishedAt > 0 {
			pubDate := time.Unix(art.PublishedAt, 0)
			cell = row.AddCell()
			cell.SetDate(pubDate)
		} else {
			row.AddCell()
		}

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
	}

	// Сохраняем в буфер
	buf := new(bytes.Buffer)
	err = file.Write(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
