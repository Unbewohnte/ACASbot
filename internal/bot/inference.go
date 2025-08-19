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
	"log"
	"strings"
)

const (
	TEMPLATE_TEXT     = "{{TEXT}}"
	TEMPLATE_OBJECT   = "{{OBJECT}}"
	TEMPLATE_METADATA = "{{METADATA}}"
)

func (bot *Bot) preparePrompt(template string, text string) string {
	prompt := strings.ReplaceAll(template, TEMPLATE_TEXT, text)
	prompt = strings.ReplaceAll(prompt, TEMPLATE_METADATA, bot.conf.Analysis.ObjectMetadata)
	prompt = strings.ReplaceAll(prompt, TEMPLATE_OBJECT, bot.conf.Analysis.Object)

	if bot.conf.Debug {
		log.Printf("Подготовленный промпт: %s", prompt)
	}

	return prompt
}

// Запрос для извлечения заголовка
func (bot *Bot) queryTitle(content string) (string, error) {
	return bot.model.Query(
		bot.preparePrompt(
			bot.conf.Ollama.Prompts.Title,
			content,
		),
	)
}

// Запрос для определения связи
func (bot *Bot) queryAffiliation(content string) (string, error) {
	return bot.model.Query(
		bot.preparePrompt(
			bot.conf.Ollama.Prompts.Affiliation,
			content,
		),
	)
}

// Запрос для определения отношения к организации
func (bot *Bot) querySentiment(content string) (string, error) {
	return bot.model.Query(
		bot.preparePrompt(
			bot.conf.Ollama.Prompts.Sentiment,
			content,
		),
	)
}

func extractSentiment(response string) string {
	response = strings.ToLower(response)

	if matchesAny(response, "позитив", "полож", "доброжелательн", "благоприятн", "поддерживающ", "дружелюбн", "восторжен", "одобритель") {
		return "Позитивный"
	}

	if matchesAny(response, "негатив", "отрицат", "критическ", "осуждающ", "агрессивн", "враждебн", "презрительн", "гнев", "недовол", "вражд") {
		return "Отрицательный"
	}

	if matchesAny(response, "нейтральн", "информационн", "объективн", "фактическ", "безоценочн", "аналитическ", "нейтрален", "нет мнения", "не выражено", "не определено") {
		return "Информационный"
	}

	return "Информационный"
}

func matchesAny(text string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(text, substr) {
			return true
		}
	}
	return false
}
