package bot

import (
	"fmt"
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
				"Обоснуй ответ только одним предложением. Формат ответа:\nОтношение: [вариант]\nОбоснование: [твое объяснение]\n\nТекст:\n%s",
			bot.conf.OrganizationName,
			content,
		)
	}

	response, err := bot.model.Query(prompt)
	if err != nil {
		errors <- fmt.Errorf("ошибка запроса отношения: %w", err)
		return
	}
	results <- response
}
