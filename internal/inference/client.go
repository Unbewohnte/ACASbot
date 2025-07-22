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
	"Unbewohnte/ACASbot/internal/similarity"
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	ollama "github.com/ollama/ollama/api"
)

type Client struct {
	ModelName      string
	EmbeddingModel string
	Client         *ollama.Client
	TimeoutSeconds uint
}

func NewClient(ollamaModel string, embeddingModel string, timeoutSeconds uint) (*Client, error) {
	inference := &Client{
		ModelName:      ollamaModel,
		EmbeddingModel: embeddingModel,
		TimeoutSeconds: timeoutSeconds,
	}

	client, err := ollama.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	inference.Client = client

	return inference, nil
}

func (i *Client) ListModels() ([]ollama.ListModelResponse, error) {
	response, err := i.Client.List(context.Background())
	if err != nil {
		return nil, err
	}

	return response.Models, nil
}

func (c *Client) Query(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.TimeoutSeconds)*time.Second,
	)
	defer cancel()

	var response strings.Builder
	err := c.Client.Generate(ctx, &ollama.GenerateRequest{
		Model:  c.ModelName,
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

func (c *Client) GetEmbedding(text string) ([]float64, error) {
	if len([]rune(text)) < 50 {
		return nil, fmt.Errorf("text too short for meaningful embedding")
	}

	// Add context for better semantic understanding
	contextualized := fmt.Sprintf("новостная статья: %s", text)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()

	req := &ollama.EmbeddingRequest{
		Model:  c.EmbeddingModel,
		Prompt: contextualized,
	}

	resp, err := c.Client.Embeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	if len(resp.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	// Make a copy of the embedding slice
	embedding := make([]float64, len(resp.Embedding))
	copy(embedding, resp.Embedding)

	// Debug: Check normalization
	var sum float64
	for _, v := range embedding {
		sum += v * v
	}
	magnitude := math.Sqrt(sum)
	fmt.Printf("Embedding magnitude before normalization: %f\n", magnitude)

	similarity.NormalizeVector(embedding)

	sum = 0
	for _, v := range embedding {
		sum += v * v
	}
	magnitude = math.Sqrt(sum)
	fmt.Printf("Embedding magnitude after normalization: %f\n", magnitude)

	return embedding, nil
}

func removeThinkBlock(input string) string {
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	return strings.TrimSpace(re.ReplaceAllString(input, ""))
}
