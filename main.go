package main

import (
	"Unbewohnte/ACASbot/bot"
	"Unbewohnte/ACASbot/conf"
	"io"
	"log"
	"os"
)

const CONFIG_NAME string = "config.json"

var CONFIG *conf.Config

func init() {
	var err error
	CONFIG, err = conf.From(CONFIG_NAME)
	if err != nil {
		log.Println("Не удалось открыть конфигурационный файл: " + err.Error() + ". Создаем новый...")
		CONFIG = conf.Default()
		err = CONFIG.Save(CONFIG_NAME)
		if err != nil {
			log.Panic("Не получилось создать новый конфигурационный файл: " + err.Error())
		}
		os.Exit(0)
	}

	if CONFIG.PushToGoogleSheet {
		file, err := os.Open(CONFIG.CredentialsFile)
		if err != nil {
			log.Panic(err)
		}
		defer file.Close()

		credentialsJSON, err := io.ReadAll(file)
		if err != nil {
			log.Panic(err)
		}

		CONFIG.SheetConfig.CredentialsJSON = credentialsJSON
	}
}

func main() {
	bot, err := bot.NewBot(CONFIG)
	if err != nil {
		log.Panic(err)
	}

	if err := bot.Start(); err != nil {
		log.Panic(err)
	}
}
