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

package inference

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	ollama "github.com/ollama/ollama/api"
)

type Inference struct {
	ModelName      string
	Client         *ollama.Client
	TimeoutSeconds uint
}

func NewInference(ollamaModel string, timeoutSeconds uint) (*Inference, error) {
	inference := &Inference{
		ModelName:      ollamaModel,
		TimeoutSeconds: timeoutSeconds,
	}

	client, err := ollama.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	inference.Client = client

	if err := inference.CheckModel(); err != nil {
		return nil, err
	}

	return inference, nil
}

func (i *Inference) CheckModel() error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(i.TimeoutSeconds)*time.Second,
	)
	defer cancel()

	modelsResp, err := i.Client.List(ctx)
	if err != nil {
		return fmt.Errorf("ошибка при запросе списка моделей: %w", err)
	}

	modelFound := false
	for _, model := range modelsResp.Models {
		if model.Name == i.ModelName {
			modelFound = true
			break
		}
	}

	if !modelFound {
		return fmt.Errorf("модель '%s' не найдена в Ollama", i.ModelName)
	}

	testCtx, testCancel := context.WithTimeout(
		context.Background(), time.Duration(i.TimeoutSeconds)*time.Second,
	)
	defer testCancel()

	testPrompt := "Ответь одним словом: работаешь?"
	var response strings.Builder
	err = i.Client.Generate(testCtx, &ollama.GenerateRequest{
		Model:  i.ModelName,
		Prompt: testPrompt,
	}, func(res ollama.GenerateResponse) error {
		response.WriteString(res.Response)
		return nil
	})

	if err != nil {
		return fmt.Errorf("тестовый запрос к модели не удался: %w", err)
	}

	log.Printf("Проверка Ollama: модель %s готова к работе (тестовый ответ: %s)", i.ModelName, removeThinkBlock(response.String()))
	return nil
}

func (i *Inference) ListModels() ([]ollama.ListModelResponse, error) {
	response, err := i.Client.List(context.Background())
	if err != nil {
		return nil, err
	}

	return response.Models, nil
}

func (i *Inference) Query(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(i.TimeoutSeconds)*time.Second,
	)
	defer cancel()

	var response strings.Builder
	err := i.Client.Generate(ctx, &ollama.GenerateRequest{
		Model:  i.ModelName,
		Prompt: prompt,
		Options: map[string]interface{}{
			"temperature": 0.2, // Для более детерминированного вывода
		},
	}, func(res ollama.GenerateResponse) error {
		response.WriteString(res.Response)
		return nil
	})

	if err != nil {
		return "", err
	}

	return removeThinkBlock(response.String()), nil
}

func removeThinkBlock(input string) string {
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	return strings.TrimSpace(re.ReplaceAllString(input, ""))
}
