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
	"fmt"
	"strings"
	"sync"
)

// Запрос для извлечения заголовка
func (bot *Bot) queryTitle(content string, wg *sync.WaitGroup, results chan<- string, errors chan<- error) {
	defer wg.Done()

	prompt := fmt.Sprintf(
		"Извлеки основной заголовок статьи из следующего текста. "+
			"Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n%s",
		content,
	)

	response, err := bot.model.Query(prompt)
	if err != nil {
		errors <- fmt.Errorf("ошибка запроса заголовка: %w", err)
		return
	}
	results <- response
}

// Запрос для определения темы
func (bot *Bot) queryTheme(content string, wg *sync.WaitGroup, results chan<- string, errors chan<- error) {
	defer wg.Done()

	prompt := fmt.Sprintf(
		"Опиши основную тему следующего текста в 1-2 предложениях. Ответ должен быть кратким и содержательным.\n\nТекст:\n%s",
		content,
	)

	response, err := bot.model.Query(prompt)
	if err != nil {
		errors <- fmt.Errorf("ошибка запроса темы: %w", err)
		return
	}
	results <- response
}

// Запрос для определения отношения к организации
func (bot *Bot) querySentiment(
	content string,
	shortAnswer bool,
	wg *sync.WaitGroup,
	results chan<- string,
	errors chan<- error,
) {
	defer wg.Done()

	var prompt string
	if shortAnswer {
		prompt = fmt.Sprintf(
			"Определи отношение к \"%s\" в следующем тексте. Варианты: положительный, информационный, отрицательный. Отвечай одним словом. В случае, если нет конкретного отношения, отвечай \"информационный\". Помни, что новости о решении проблем - позитивны, а новости о проишествиях скорее информационны, чем отрицательны.\n\nТекст: \n%s",
			bot.conf.OrganizationName,
			content,
		)
	} else {
		prompt = fmt.Sprintf(
			"Определи отношение к \"%s\" в следующем тексте. Варианты: положительный, информационный, отрицательный. В случае, если нет конкретного отношения, отвечай \"информационный\". Помни, что новости о решении проблем - позитивны, а новости о проишествиях скорее информационны, чем отрицательны. "+
				"Обоснуй ответ только одним предложением. Формат ответа:\n[отношение]\nОбоснование: [твое объяснение]\n\nТекст:\n%s",
			bot.conf.OrganizationName,
			content,
		)
	}

	response, err := bot.model.Query(prompt)
	if err != nil {
		errors <- fmt.Errorf("ошибка запроса отношения: %w", err)
		return
	}

	if shortAnswer {
		// Гарантируем ответ одним словом
		rsp := strings.ToLower(response)
		if strings.Contains(rsp, "положительный") {
			response = "Положительный"
		} else if strings.Contains(rsp, "отрицательный") {
			response = "Отрицательный"
		} else {
			response = "Информационный"
		}
	}

	results <- response
}
