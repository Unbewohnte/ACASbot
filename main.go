package main

import (
	"Unbewohnte/ACATbot/bot"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	telegramAPIToken = flag.String("tgToken", "", "API токен бота в телеграм")
	organizationName = flag.String("organization", "Человечество", "Организация/человек, отношение к которой/которому будет определяться")
	ollamaModel      = flag.String("model", "lakomoor/vikhr-llama-3.2-1b-instruct:1b", "Имя LLM, используемое ollama для интерференции")
	maxContentSize   = flag.Uint("maxContentSize", 10000, "Максимальное количество символов статьи для разбора")
	debug            = flag.Bool("debug", true, "Печатать больше информации во время работы")
	extractText      = flag.String("extractText", "", "Ссылка на публикацию в Интернете. При непустом значении однажды извелечен данные и выйдет")
	fullAnalysis     = flag.Bool("fullAnalysis", true, "Проводить полный анализ статей")
)

func main() {
	flag.Parse()

	bot, err := bot.NewBot(
		bot.NewConfig(
			*telegramAPIToken,
			*organizationName,
			*ollamaModel,
			*maxContentSize,
			*debug,
			*fullAnalysis,
		),
	)
	if err != nil {
		log.Panic(err)
	}

	if strings.TrimSpace(*extractText) != "" {
		text, err := bot.ExtractWebContent(*extractText)
		if err != nil {
			fmt.Printf("Не удалось найти текст статьи: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("[Найденнный текст страницы]: %s\n", text)
		os.Exit(0)
	}

	if err := bot.Start(); err != nil {
		log.Panic(err)
	}
}
