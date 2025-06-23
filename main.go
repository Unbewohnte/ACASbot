package main

import (
	"Unbewohnte/ACATbot/bot"
	"Unbewohnte/ACATbot/conf"
	"Unbewohnte/ACATbot/spreadsheet"
	"flag"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var (
	debug = flag.Bool("debug", true, "Печатать больше информации во время работы")
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Panic(err)
	}

	flag.Parse()
}

func main() {
	var credentialsJSON []byte
	if os.Getenv("PUSH_TO_SHEET") == "true" {
		file, err := os.Open(os.Getenv("SHEET_CREDENTIALS_FILE"))
		if err != nil {
			log.Panic(err)
		}
		defer file.Close()

		credentialsJSON, err = io.ReadAll(file)
		if err != nil {
			log.Panic(err)
		}
	}

	maxContentSize, err := strconv.Atoi(os.Getenv("MAX_CONTENT_SIZE"))
	if err != nil {
		log.Panic(err)
	}

	allowedUsers := []int64{}
	for _, idStr := range strings.Split(os.Getenv("ALLOWED_TG_USERS_IDS"), ",") {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			log.Panic("Найден неверный ID пользователя")
		}
		allowedUsers = append(allowedUsers, id)
	}

	bot, err := bot.NewBot(
		conf.NewConfig(
			os.Getenv("TELEGRAM_TOKEN"),
			os.Getenv("ORGANIZATION"),
			os.Getenv("OLLAMA_MODEL"),
			uint(maxContentSize),
			*debug,
			os.Getenv("FULL_ANALYSIS") == "true",
			os.Getenv("PUSH_TO_SHEET") == "true",
			spreadsheet.NewConfig(
				credentialsJSON,
				os.Getenv("SHEET_ID"),
				os.Getenv("SHEET_NAME"),
			),
			os.Getenv("IS_PUBLIC") == "true",
			allowedUsers,
		),
	)
	if err != nil {
		log.Panic(err)
	}

	if err := bot.Start(); err != nil {
		log.Panic(err)
	}
}
