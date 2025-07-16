# ACASbot
## Article Context And Sentiment bot

# RU

## Общее

Это специфический телеграм бот для упрощения анализа статей на предмет отношения к определенной организации, используя запросы к локальной LLM, запущенной на платформе ollama.

Бот обладает несколькими режимами и настройками. Для конкретной информации лучше обратиться к самому боту с командой `help` или `help [команда]`.

Имеющийся функционал:
- изменение имени организации;
- возможность проведения полного или краткого анализа;
- включение/выключение публичности бота;
- добавление/удаление пользователей с доступом к боту;
- изменение запросов к модели;
- смена одной установленной локальной модели на другую на лету;
- получение информации статьи при помощи headless браузера с фоллбэком на обычный запрос;
- отправка результатов анализа в Google таблицу и/или локальный XLSX файл.

Бот способен автоматически добавлять в Google таблицу результирующую информацию в формате следующей строки: дата публикации, источник (доменное имя), краткое описание (заголовок), URL, тип отношения к организаии (информационный, отрицательный, положительный).

Настройка автоматической работы с Google таблицой требует предварительной настройки: создания Google проекта, включения соответствующего API, получения сервисного аккаунта и ключа доступа со стороны гугла, а также ID самой таблицы (из адресной строки) и наименования листа. Бот сам найдет последнюю запись в вышеописанной структуре и добавит новую при завершении анализа очередной статьи.

Нынешняя реализация подразумевает, что ботом пользуюется один, или несколько связанных друг с другом людей, так как настройка глобальна, - изменения, произведенные одним пользователем, отражаются на все последующие запросы без привязки к конкретным лицам.


## Программные зависимости

- ollama с локальной LLM (рекомендация: возьмите модель с количеством параметров >1 миллиарда);


## Системные требования

- Система: Windows, Linux, Mac (поддерживаемые ollama. Сам бот является кроссплатформенным)
- Архитектура: amd64
- ОЗУ, ЦП, ГПУ: В зависимости от выбранной LLM

## Настройка

Для работы необходимы:

- Телеграм токен;
- Работающий сервис ollama с доступной для работы моделью;
- Файл доступа для сервисного аккаунта от Google;
- Информация о Google таблице. 

### Telegram token

Зарегистрируйте бота у @BotFather и получите токен.

### ollama модель

Подойдет любая модель, способная работать в режиме помощника и воспринимающая требуемый язык.

Пример: ollama pull bambucha/saiga-llama3:latest

### Google таблица

- Войдите в [Google Cloud](https://console.cloud.google.com), создайте проект;
- В `API & Services` включите `Google Sheets API`;
- В `Credentials`создайте сервисный аккаунт и получите JSON файл доступа;
- Создайте Google таблицу, добавьте сервисный аккаунт в роли правщика;
- Скопируйте ID таблицы (предпоследняя, длинная часть URL до /view или /edit, состоящая из различных символов и цифр);
- Скопируйте название листа (Обычно `Sheet 1` или `Лист 1`).

При первом запуске бота он создаст в рабочей директории файл настройки с данными по умолчанию. Нынешняя конфигурация выглядит примерно так:

```json
{
	"telegram": {
		"api_token": "tg_api_token",
		"is_public": true,
		"allowed_user_ids": []
	},
	"ollama": {
		"general_model": "bambucha/saiga-llama3:latest",
		"query_timeout_seconds": 600,
		"prompts": {
			"affiliation": "Опиши одним предложением, какая информация в тексте имеет отношение к \"{{OBJECT}}\". Если не имеет, ответь только \"Связи нет\"\n\nТекст:\n{{TEXT}}",
			"sentiment_short": "Определи отношение к \"{{OBJECT}}\" в следующем тексте. Варианты: положительный, информационный, отрицательный. Отвечай одним словом. В случае, если нет конкретного отношения, отвечай \"информационный\".\n\nТекст: \n{{TEXT}}",
			"sentiment_long": "Определи отношение к \"{{OBJECT}}\" в тексте. Варианты: положительный, информационный, отрицательный. В случае, если нет конкретного отношения, отвечай \"информационный\". Обоснуй ответ только одним предложением. Формат ответа:\n[отношение одним словом]\nОбоснование: [твое объяснение]\n\nТекст:\n{{TEXT}}",
			"title": "Извлеки основной заголовок статьи из следующего текста. Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n{{TEXT}}"
		},
		"embedding_model": "bge-m3:latest"
	},
	"sheets": {
		"push_to_google_sheet": true,
		"google": {
			"config": {
				"credentails": null,
				"spreadsheet_id": "spreadsheet_id",
				"sheet_name": "Sheet 1"
			},
			"credentials_file": "secret.json"
		}
	},
	"analysis": {
		"object": "Люди, жители",
		"object_metadata": "",
		"max_content_size": 8000,
		"save_similar_articles": true
	},
	"debug": false,
	"database": {
		"file": "ACASBOT.sqlite3"
	}
}
```

- Токен телеграма вносится в `api_token`;
- LLM в формате, воспринимаемым ollama вносится в `model`;
- Путь к файлу доступа сервисного аккаунта пишется в `credentials_file`;
- Идентификатор таблицы - `spreadsheet_id`;
- Наименование листа - `sheet_name`.

На этом настройка может быть окончена, остальное можно контролировать уже используя самого бота.

Так как промпты вынесены в конфигурационный файл, можно контролировать язык ответа от LLM.

## Использование

Пример:


Запрос:
`do https://somenewswebsite.org/news/article12345`

Ответ:
```
📋 Результаты анализа

Заголовок: заголовок статьи

Тема: Основная тема текста

Отношение: Информационный
Обоснование: Текст не несет конкретной оценки.
```

При правильной настройке и включенной опции `push_to_google_sheet`, информация будет добавлена и в Google таблицу.


## Лицензия

GPLv3. Для большей информации см. `COPYING`.


# EN

## General

This is a very specific telegram bot suited for the analysis of articles for their relation to a certain organization using requests to a local LLM running via ollama.

The bot has several modes and settings. For specific information, it is better to contact the bot itself with the `help` command.

Available functionality:
- changing the organization name;
- the ability to conduct a full or brief analysis;
- enabling/disabling the bot's publicity;
- adding/removing users with access to the bot;
- adding/removing users with access to the bot;
- changing model prompts;
- changing one installed local model to another on the fly;
- getting information using a standalone browser with fallback on a regular request;
- sending analysis results to a Google spreadsheet and/or a local XLSX file.

The bot can automatically add the resulting information to the Google table in the following line format: publication date, source (domain name), short description (title), URL, type of relation to the organization (informational, negative, positive).

Setting up automatic work with the Google table requires configuration: creating a Google project, enabling the corresponding API, obtaining a service account and an access key from Google, as well as the ID of the table itself (from the address bar) and the name of the sheet. The bot itself will find the last entry in the above-described structure and add a new one when the analysis of the next article is completed.

The current implementation implies that the bot is used by one or several people connected to each other, since the setting is global - changes made by one user are reflected in all subsequent requests without reference to specific persons.

## Software dependencies

- ollama with local LLM (recommendation: take a model with the number of parameters > 1b);

## System requirements

- System: Windows, Linux, Mac (supported by ollama. The bot itself is cross-platform)
- Architecture: amd64
- RAM, CPU, GPU: Depending on the selected LLM

## Setup

For work you need:

- Telegram token;
- Working ollama service with an available model;
- Access file for the service account from Google;
- Information about the Google table.

### Telegram token

Register the bot with @BotFather and get a token.

### ollama model

Any model that can work in assistant mode and understands the required language will do.

Example: ollama pull bambucha/saiga-llama3:latest

### Google spreadsheet

- Log in to [Google Cloud](https://console.cloud.google.com), create a project;
- In `API & Services` enable `Google Sheets API`;
- In `Credentials` create a service account and get a JSON access file;
- Create a Google spreadsheet, add the service account as an editor;
- Copy the spreadsheet ID (the penultimate, long part of the URL before /view or /edit, consisting of various symbols and numbers);
- Copy the sheet name (usually `Sheet 1` or `Sheet 1`).

When you first run the bot, it will create a configuration file with default data in the working directory. The current configuration looks something like this:

```json
{
	"telegram": {
		"api_token": "tg_api_token",
		"is_public": true,
		"allowed_user_ids": []
	},
	"ollama": {
		"general_model": "bambucha/saiga-llama3:latest",
		"query_timeout_seconds": 600,
		"prompts": {
			"affiliation": "Опиши одним предложением, какая информация в тексте имеет отношение к \"{{OBJECT}}\". Если не имеет, ответь только \"Связи нет\"\n\nТекст:\n{{TEXT}}",
			"sentiment_short": "Определи отношение к \"{{OBJECT}}\" в следующем тексте. Варианты: положительный, информационный, отрицательный. Отвечай одним словом. В случае, если нет конкретного отношения, отвечай \"информационный\".\n\nТекст: \n{{TEXT}}",
			"sentiment_long": "Определи отношение к \"{{OBJECT}}\" в тексте. Варианты: положительный, информационный, отрицательный. В случае, если нет конкретного отношения, отвечай \"информационный\". Обоснуй ответ только одним предложением. Формат ответа:\n[отношение одним словом]\nОбоснование: [твое объяснение]\n\nТекст:\n{{TEXT}}",
			"title": "Извлеки основной заголовок статьи из следующего текста. Ответ должен содержать только заголовок без дополнительных комментариев.\n\nТекст:\n{{TEXT}}"
		},
		"embedding_model": "bge-m3:latest"
	},
	"sheets": {
		"push_to_google_sheet": true,
		"google": {
			"config": {
				"credentails": null,
				"spreadsheet_id": "spreadsheet_id",
				"sheet_name": "Sheet 1"
			},
			"credentials_file": "secret.json"
		}
	},
	"analysis": {
		"object": "Люди, жители",
		"object_metadata": "",
		"max_content_size": 8000,
		"save_similar_articles": true
	},
	"debug": false,
	"database": {
		"file": "ACASBOT.sqlite3"
	}
}
```

- Telegram token goes into `api_token`;
- LLM in the format which is understood by ollama is entered into `model`;
- Path to the service account access file is written in `credentials_file`;
- Spreadsheet ID - `spreadsheet_id`;
- Sheet name - `sheet_name`.

That's it for the setup, the rest can be controlled and changed using the bot itself.

Since the prompts are moved to the configuration file, you can control the language of the response from LLM.

## Usage

Example:

Request:
`do https://somenewswebsite.org/news/article12345`

Response:

```
📋 Analysis results

Title: article title

Topic: Main topic of the text

Relation: Informational
Justification: The text does not carry a specific assessment.
```

If configured correctly and the `push_to_google_sheet` option is enabled, the information will be added to the Google sheet.

## License

GPLv3. For more information, see `COPYING`.