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

package main

import (
	"Unbewohnte/ACASbot/internal/bot"
	"io"
	"log"
	"os"
)

const CONFIG_NAME string = "config.json"

var (
	CONFIG *bot.Config
)

func init() {
	var err error
	CONFIG, err = bot.ConfigFrom(CONFIG_NAME)
	if err != nil {
		log.Println("Не удалось открыть конфигурационный файл: " + err.Error() + ". Создаем новый...")
		CONFIG = bot.DefaultConfig()
		err = CONFIG.Save(CONFIG_NAME)
		if err != nil {
			log.Panic("Не получилось создать новый конфигурационный файл: " + err.Error())
		}
		os.Exit(0)
	}

	logsFile, err := os.Create(CONFIG.LogsFile)
	if err != nil {
		log.Panic("Не получилось создать файл логов: " + err.Error())
	}
	log.SetOutput(io.MultiWriter(logsFile, os.Stdout))

	if CONFIG.Sheets.PushToGoogleSheet {
		file, err := os.Open(CONFIG.Sheets.Google.CredentialsFile)
		if err != nil {
			log.Panic(err)
		}
		defer file.Close()

		credentialsJSON, err := io.ReadAll(file)
		if err != nil {
			log.Panic(err)
		}

		CONFIG.Sheets.Google.Config.CredentialsJSON = credentialsJSON
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
