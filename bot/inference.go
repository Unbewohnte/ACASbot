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
	prompt = strings.ReplaceAll(prompt, TEMPLATE_METADATA, bot.conf.ObjectMetadata)
	prompt = strings.ReplaceAll(prompt, TEMPLATE_OBJECT, bot.conf.Object)

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
func (bot *Bot) querySentiment(
	content string,
	shortAnswer bool,
) (string, error) {
	var prompt string
	if shortAnswer {
		prompt = bot.preparePrompt(
			bot.conf.Ollama.Prompts.SentimentShort,
			content,
		)
	} else {
		prompt = bot.preparePrompt(
			bot.conf.Ollama.Prompts.SentimentLong,
			content,
		)
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
