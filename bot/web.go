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
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	trafilatura "github.com/markusmobius/go-trafilatura"
)

type ArticleContent struct {
	Title   string
	Content string
	Success bool
	PubDate *time.Time
}

type ArticleAnalysis struct {
	URL            string
	Content        ArticleContent
	TitleFromModel string
	Affiliation    string
	Sentiment      string
	Justification  string
	Errors         []error
}

func (bot *Bot) ExtractWebContent(articleURL string) (ArticleContent, error) {
	var htmlData []byte
	var err error

	htmlData, err = bot.extractWithHeadlessBrowser(articleURL)
	if err != nil {
		log.Printf("Не получилось получить данные при помощи headless браузера: %s. Откат к обычному запросу...", err)

		htmlData, err = bot.extractWithoutHeadless(articleURL)
		if err != nil {
			log.Printf("Не получилось получить данные при помощи обычного запроса: %s", err)
			return ArticleContent{}, err
		}
	}

	html := string(htmlData)

	// Используем trafilatura вместо readability
	parseOpts := trafilatura.Options{
		ExcludeTables:   false,
		IncludeLinks:    false,
		Deduplicate:     true,
		ExcludeComments: true,
		EnableFallback:  true,
	}

	doc, err := trafilatura.Extract(strings.NewReader(html), parseOpts)
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка извлечения контента: %w", err)
	}

	if doc != nil && doc.ContentText != "" {
		var pubTime *time.Time
		if !doc.Metadata.Date.IsZero() {
			pubTime = &doc.Metadata.Date
		}

		return ArticleContent{
			Title:   doc.Metadata.Title,
			Content: doc.ContentText,
			Success: true,
			PubDate: pubTime,
		}, nil
	}

	// Пробуем кастомный парсер
	queryDoc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка парсинга HTML: %w", err)
	}

	return bot.extractCustomContent(queryDoc)
}

var userAgents = []string{
	// Современные Chrome (Windows, Mac, Linux)
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",

	// Firefox
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0",

	// Safari
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",

	// Edge
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",

	// Мобильные
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Linux; Android 14; SM-S901B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Mobile Safari/537.36",
}

func (bot *Bot) getRandomUserAgent() string {
	return userAgents[rand.Intn(len(userAgents))]
}

func (bot *Bot) extractWithHeadlessBrowser(articleURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(bot.getRandomUserAgent()),
		chromedp.WindowSize(1280, 800),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("ignore-certificate-errors", true),

		// Блокируем ненужные ресурсы
		chromedp.Flag("blink-settings", "imagesEnabled=false,stylesheetEnabled=false,scriptEnabled=true"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	var htmlContent string
	actions := []chromedp.Action{
		network.Enable(),
		network.SetExtraHTTPHeaders(map[string]interface{}{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7",
			"Cache-Control":   "no-cache",
		}),
		// Блокируем загрузку тяжелых ресурсов
		network.SetBlockedURLs([]string{
			"*.png", "*.jpg", "*.jpeg", "*.gif", "*.svg", "*.webp",
			"*.css", "*.woff", "*.woff2", "*.ttf", "*.eot",
			"*.mp4", "*.webm", "*.ogg", "*.avi",
		}),
		chromedp.Navigate(articleURL),

		// Обнуляем флаг webdriver
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(`delete navigator.__proto__.webdriver`).Do(ctx)
			return err
		}),

		chromedp.Sleep(3 * time.Second), // Минимальная задержка для старта JS
		chromedp.MouseEvent(input.MouseMoved, 640, 400),
		chromedp.ScrollIntoView("body"),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &htmlContent),
	}

	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		browserCtx, browserCancel := chromedp.NewContext(allocCtx)
		defer browserCancel()

		if err = chromedp.Run(browserCtx, actions...); err == nil {
			break
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}

		time.Sleep(time.Duration(attempt*2) * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("не удалось загрузить страницу после 3 попыток: %w", err)
	}

	return []byte(htmlContent), nil
}

func shouldBlockRequest(url string) bool {
	blockedPatterns := []string{
		".png", ".jpg", ".jpeg", ".gif", ".webp",
		".css", ".woff", ".woff2", ".ttf",
		".mp4", ".avi", ".webm", ".mov",
		"doubleclick.net", "googleadservices.com",
		"analytics", "tracking", "metrics",
	}

	for _, pattern := range blockedPatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}

func (bot *Bot) extractWithoutHeadless(articleURL string) ([]byte, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания cookie jar: %w", err)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header = via[0].Header.Clone()
			return nil
		},
	}

	req, err := http.NewRequest("GET", articleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	bot.setAdvancedHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки страницы: %w", err)
	}
	defer resp.Body.Close()

	var reader io.Reader

	// Проверяем Content-Encoding и распаковываем при необходимости
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	case "deflate":
		reader = flate.NewReader(resp.Body)
		defer reader.(io.ReadCloser).Close()
	default:
		reader = resp.Body
	}

	// Читаем тело ответа
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела ответа: %w", err)
	}

	// Проверяем, что это текст
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "text/plain") {
		// Попробуем определить кодировку по содержимому
		if !utf8.Valid(bodyBytes) {
			return nil, fmt.Errorf("получены бинарные данные, не похожие на текст")
		}
	}

	return bodyBytes, nil
}

func (bot *Bot) setAdvancedHeaders(req *http.Request) {
	headers := map[string]string{
		"User-Agent":                bot.getRandomUserAgent(),
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Accept-Language":           "ru-RU,ru;q=0.8,en-US;q=0.5,en;q=0.3",
		"Accept-Encoding":           "gzip, deflate",
		"Connection":                "keep-alive",
		"Referer":                   "https://www.google.com/",
		"DNT":                       "1",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func (bot *Bot) extractCustomContent(doc *goquery.Document) (ArticleContent, error) {
	// Сначала пробуем структурированный подход
	if content := bot.extractStructuredContent(doc); content.Success {
		return content, nil
	}

	// Затем fallback-метод
	content, err := bot.extractFallbackContent(doc)
	if err != nil {
		return ArticleContent{}, err
	}

	return ArticleContent{
		Content: content,
		Success: false,
	}, nil
}

func (bot *Bot) extractStructuredContent(doc *goquery.Document) ArticleContent {
	articleSelection := doc.Find("article, main, .article, .post, .content")
	if articleSelection.Length() == 0 {
		return ArticleContent{Success: false}
	}

	var title string
	for _, selector := range []string{"h1", "h2", ".title", ".article-title"} {
		if title == "" {
			title = strings.TrimSpace(articleSelection.Find(selector).First().Text())
		}
	}

	content := strings.TrimSpace(articleSelection.Text())
	content = strings.Join(strings.Fields(content), " ")

	if len(content) < 100 || uint(len(content)) > bot.conf.MaxContentSize {
		return ArticleContent{Success: false}
	}

	return ArticleContent{
		Title:   title,
		Content: content,
		Success: true,
	}
}

func (bot *Bot) extractFallbackContent(doc *goquery.Document) (string, error) {
	// Очистка документа
	doc.Find("script, style, noscript, iframe, nav, footer").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	// Поиск основного контента
	mainContent := ""
	doc.Find("p, div, article").Each(func(i int, s *goquery.Selection) {
		if text := strings.TrimSpace(s.Text()); len(text) > len(mainContent) {
			mainContent = text
		}
	})

	if len(mainContent) < 500 {
		mainContent = strings.TrimSpace(doc.Find("body").Text())
	}

	mainContent = strings.Join(strings.Fields(mainContent), " ")
	if len(mainContent) < 100 {
		if bot.conf.Debug {
			log.Printf("Недостаточно текста: %s", mainContent)
		}
		return "", fmt.Errorf("недостаточно текста")
	}

	return mainContent, nil
}

func cleanContent(content string) string {
	// 1. Удаляем все управляющие символы и непечатаемые символы
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' ' // Заменяем на обычный пробел
		}
		if unicode.IsControl(r) || unicode.IsMark(r) {
			return -1 // Удаляем
		}
		if r < 32 || r > 126 && r < 160 {
			return -1 // Удаляем нестандартные символы
		}
		return r
	}, content)

	// 2. Заменяем различные варианты пробелов на обычный пробел
	cleaned = regexp.MustCompile(`[\s\p{Zs}]+`).ReplaceAllString(cleaned, " ")

	// 3. Удаляем лишние пробелы вокруг пунктуации
	cleaned = regexp.MustCompile(`\s+([.,!?;:)]+)`).ReplaceAllString(cleaned, "$1")
	cleaned = regexp.MustCompile(`([([{])\s+`).ReplaceAllString(cleaned, "$1")

	// 4. Удаляем "мусорные" последовательности символов
	cleaned = regexp.MustCompile(`[=+*_\-~]{3,}`).ReplaceAllString(cleaned, " ")   // Разделители
	cleaned = regexp.MustCompile(`[\p{So}\p{Sk}]+`).ReplaceAllString(cleaned, " ") // Символы и модификаторы

	// 5. Удаляем одиночные символы кроме букв и цифр
	cleaned = regexp.MustCompile(`(^|\s)[^а-яА-Яa-zA-Z0-9](\s|$)`).ReplaceAllString(cleaned, " ")

	// 6. Удаляем повторяющиеся пробелы
	cleaned = regexp.MustCompile(` {2,}`).ReplaceAllString(cleaned, " ")

	// 7. Удаляем пробелы в начале и конце
	cleaned = strings.TrimSpace(cleaned)

	// 8. Восстанавливаем стандартные кавычки
	cleaned = strings.ReplaceAll(cleaned, "«", "\"")
	cleaned = strings.ReplaceAll(cleaned, "»", "\"")
	cleaned = strings.ReplaceAll(cleaned, "“", "\"")
	cleaned = strings.ReplaceAll(cleaned, "”", "\"")

	// 9. Удаляем оставшиеся одиночные специальные символы
	cleaned = regexp.MustCompile(`\s[^а-яА-Яa-zA-Z0-9\s]\s`).ReplaceAllString(cleaned, " ")

	return cleaned
}

type QueryResult struct {
	Type    string
	Content string
}

func (bot *Bot) analyzeArticle(url string) (*ArticleAnalysis, error) {
	articleContent, err := bot.ExtractWebContent(url)
	if err != nil {
		return nil, err
	}

	articleContent.Content = cleanContent(articleContent.Content)

	result := &ArticleAnalysis{
		URL:     url,
		Content: articleContent,
	}

	if bot.conf.Debug {
		status := "структурированный"
		if !result.Content.Success {
			status = "фолбэк"
		}
		log.Printf("Использован %s метод. Заголовок: %s; Содержимое: %s",
			status,
			result.Content.Title,
			result.Content.Content,
		)
	}

	// Ограничение размера контента
	if uint(len([]rune(result.Content.Content))) > bot.conf.MaxContentSize {
		result.Content.Content = string([]rune(result.Content.Content)[:bot.conf.MaxContentSize])
		if bot.conf.Debug {
			log.Printf("Урезано до: %s\n", result.Content.Content)
		}
	}

	var wg sync.WaitGroup
	results := make(chan QueryResult, 3)
	errors := make(chan error, 3)

	// Типы запросов
	const (
		QueryTitle       = "title"
		QueryAffiliation = "affiliation"
		QuerySentiment   = "sentiment"
	)

	needTitle := !result.Content.Success || result.Content.Title == ""
	if needTitle {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := bot.queryTitle(result.Content.Content)
			if err != nil {
				errors <- fmt.Errorf("заголовок: %w", err)
				return
			}
			results <- QueryResult{Type: QueryTitle, Content: response}
		}()
	}

	if bot.conf.FullAnalysis {
		wg.Add(2)
		go func() {
			defer wg.Done()
			response, err := bot.queryAffiliation(result.Content.Content)
			if err != nil {
				errors <- fmt.Errorf("тема: %w", err)
				return
			}
			results <- QueryResult{Type: QueryAffiliation, Content: response}
		}()
		go func() {
			defer wg.Done()
			response, err := bot.querySentiment(result.Content.Content, false)
			if err != nil {
				errors <- fmt.Errorf("отношение: %w", err)
				return
			}
			results <- QueryResult{Type: QuerySentiment, Content: response}
		}()
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := bot.querySentiment(result.Content.Content, true)
			if err != nil {
				errors <- fmt.Errorf("отношение: %w", err)
				return
			}
			results <- QueryResult{Type: QuerySentiment, Content: extractSentiment(response)}
		}()
	}

	// Обработка результатов
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Собираем результаты
	for res := range results {
		switch res.Type {
		case QueryTitle:
			result.TitleFromModel = res.Content
		case QueryAffiliation:
			result.Affiliation = res.Content
		case QuerySentiment:
			// Парсим структурированный ответ
			parts := strings.SplitN(res.Content, "\n", 2)
			if len(parts) > 0 {
				result.Sentiment = extractSentiment(strings.TrimSpace(parts[0]))
			}
			if len(parts) > 1 {
				result.Justification = strings.TrimSpace(parts[1])
			}
		}
	}

	// Собираем ошибки
	for err := range errors {
		result.Errors = append(result.Errors, err)
	}

	return result, nil
}
