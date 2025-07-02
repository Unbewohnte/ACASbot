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
	"log"
	"strings"
)

// Запрос для извлечения заголовка
func (bot *Bot) queryTitle(content string) (string, error) {
	prompt := fmt.Sprintf(
		"Извлеки основной заголовок статьи из следующего текста. "+
			"Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n%s",
		content,
	)

	return bot.model.Query(prompt)
}

// Запрос для определения связи
func (bot *Bot) queryAffiliation(content string) (string, error) {
	prompt := fmt.Sprintf(
		"Опиши одним предложением, какое отношение \"%s\" (дополнительная информация: %s) имеет к тексту, и имеет ли вообще.\n\nТекст:\n%s",
		bot.conf.OrganizationName,
		bot.conf.OrganizationMetadata,
		content,
	)

	return bot.model.Query(prompt)
}

// Запрос для определения отношения к организации
func (bot *Bot) querySentiment(
	content string,
	shortAnswer bool,
) (string, error) {
	var prompt string
	if shortAnswer {
		prompt = fmt.Sprintf(
			"Определи отношение к \"%s\" (дополнительная информация: %s) в следующем тексте. Варианты: положительный, информационный, отрицательный. Отвечай одним словом. В случае, если нет конкретного отношения, отвечай \"информационный\".\n\nТекст: \n%s",
			bot.conf.OrganizationName,
			bot.conf.OrganizationMetadata,
			content,
		)
	} else {
		prompt = fmt.Sprintf(
			"Определи отношение к \"%s\" (дополнительная информация: %s) в тексте. Варианты: положительный, информационный, отрицательный. В случае, если нет конкретного отношения, отвечай \"информационный\". "+
				"Обоснуй ответ только одним предложением. Формат ответа:\n[отношение одним словом]\nОбоснование: [твое объяснение]\n\nТекст:\n%s",
			bot.conf.OrganizationName,
			bot.conf.OrganizationMetadata,
			content,
		)
	}

	if bot.conf.Debug {
		log.Printf("Sentiment prompt: %s", prompt)
	}

	return bot.model.Query(prompt)
}

func extractSentiment(response string) string {
	response = strings.ToLower(response)

	switch {
	case strings.Contains(response, "позитив"):
		return "Позитивный"
	case strings.Contains(response, "негатив") || strings.Contains(response, "отрицат"):
		return "Отрицательный"
	default:
		return "Информационный"
	}
}

func cleanTheme(response string) string {
	if strings.HasPrefix(response, "Тема:") {
		return strings.TrimSpace(response[5:])
	}
	return response
}
