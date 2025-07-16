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
	"Unbewohnte/ACASbot/internal/article"
	"Unbewohnte/ACASbot/internal/similarity"
	"fmt"
	"sort"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Левенштейн
func minDistance(a, b string) int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + min(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
			}
		}
	}
	return dp[m][n]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func (bot *Bot) findSimilarCommands(input string) []string {
	type cmdDistance struct {
		name     string
		distance int
	}

	var distances []cmdDistance
	for _, cmd := range bot.commands {
		dist := minDistance(input, cmd.Name)
		distances = append(distances, cmdDistance{cmd.Name, dist})
	}

	sort.Slice(distances, func(i, j int) bool {
		return distances[i].distance < distances[j].distance
	})

	var suggestions []string
	for i := 0; i < 3 && i < len(distances); i++ {
		suggestions = append(suggestions, distances[i].name)
	}

	return suggestions
}

func (bot *Bot) saveNewArticle(art *article.Article, embedding []float64, sourceURL string) error {
	newArticle := &article.Article{
		Content:       art.Content,
		Title:         art.Title,
		Embedding:     embedding,
		SourceURL:     sourceURL,
		CreatedAt:     time.Now().Unix(),
		PublishedAt:   art.PublishedAt,
		Citations:     art.Citations,
		SimilarURLs:   art.SimilarURLs,
		Affiliation:   art.Affiliation,
		Sentiment:     art.Sentiment,
		Justification: art.Justification,
	}

	return bot.conf.GetDB().SaveArticle(newArticle)
}

func (bot *Bot) generateDuplicatesMessage(similar []article.Article) string {
	if len(similar) == 0 {
		return ""
	}

	msgText := "⚠️ Найдены тематически похожие статьи:\n"
	for i, art := range similar {
		msgText += fmt.Sprintf("%d. [\"%s\"](%s)\n", i+1, art.Title, art.SourceURL)
		msgText += fmt.Sprintf("- Добавлена: %s\n", time.Unix(art.CreatedAt, 0).Format("2006-01-02 15:04"))
		msgText += fmt.Sprintf("- Общая схожесть: %.0f%%\n", art.TrueSimilarity*100)
		msgText += fmt.Sprintf("-- Схожесть текста: %.0f%%\n", similarity.CalculateEnhancedTextSimilarity(art.Content, art.Content)*100)
		msgText += fmt.Sprintf("-- Схожесть векторов: %.0f%%\n", art.Similarity*100)
		msgText += fmt.Sprintf("- Цитирований: %d\n", art.Citations)

	}
	return msgText
}

func (bot *Bot) sendError(chatID int64, text string, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, "❌ "+text)
	msg.ReplyToMessageID = replyTo
	bot.api.Send(msg)
}

func (bot *Bot) sendSuccess(chatID int64, text string, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, "✅ "+text)
	msg.ReplyToMessageID = replyTo
	bot.api.Send(msg)
}

func (bot *Bot) notifyExactDuplicate(message *tgbotapi.Message, existingArticle *article.Article) {
	msgText := fmt.Sprintf(`
❌ Точный дубликат уже существует!

Оригинал:
- Заголовок: %s
- URL: %s
- Добавлен: %s
- Цитирований: %d
`,
		existingArticle.Title,
		existingArticle.SourceURL,
		time.Unix(existingArticle.CreatedAt, 0).Format("2006-01-02 15:04"),
		existingArticle.Citations,
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}
