package main

import (
	"Unbewohnte/ACATbot/bot"
	"flag"
	"log"
)

var (
	telegramAPIToken = flag.String("tgToken", "token", "API токен бота в телеграм")
	organizationName = flag.String("organization", "Человечество", "Организация/человек, отношение к которой/которому будет определяться")
	ollamaModel      = flag.String("model", "lakomoor/vikhr-llama-3.2-1b-instruct:1b", "Имя LLM, используемое ollama для инференции")
	maxContentSize   = flag.Uint("maxContentSize", 10000, "Максимальное количество символов статьи для разбора")
	debug            = flag.Bool("debug", true, "Печатать больше информации во время работы")
	fullAnalysis     = flag.Bool("fullAnalysis", false, "Проводить полный анализ статей")
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

	if err := bot.Start(); err != nil {
		log.Panic(err)
	}
}
