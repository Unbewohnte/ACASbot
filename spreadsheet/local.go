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
	"log"
	"sync"

	"github.com/tealeg/xlsx/v3"
)

var saveMutex sync.Mutex = sync.Mutex{}

func AppendToLocalSheet(fileName string, entry *SheetEntry) error {
	saveMutex.Lock()
	defer saveMutex.Unlock()

	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		// Создаем новый
		file = xlsx.NewFile()
	}

	sheet, ok := file.Sheet["Результаты"]
	if !ok {
		var err error
		sheet, err = file.AddSheet("Результаты")
		if err != nil {
			return err
		}

		// Добавляем заголовки
		headerRow := sheet.AddRow()
		headers := []string{"Дата", "Источник", "Заголовок", "URL", "Примечание", "Тон"}
		for _, h := range headers {
			cell := headerRow.AddCell()
			cell.Value = h
		}
	}

	row := sheet.AddRow()

	// Дата публикации
	dateCell := row.AddCell()
	dateCell.SetDate(entry.PublicationDate)

	// Источник
	row.AddCell().SetString(entry.Source)

	// Заголовок
	row.AddCell().SetString(entry.Summary)

	// URL
	urlCell := row.AddCell()
	urlCell.SetString(entry.URL)

	// Примечание
	row.AddCell().SetString(entry.Note)

	// Тон
	row.AddCell().SetString(entry.SentimentType)

	err = file.Save(fileName)
	if err != nil {
		log.Printf("Err: %s", err.Error())
		return err
	}

	return nil
}
