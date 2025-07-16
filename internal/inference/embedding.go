package inference

type EmbeddingRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Options struct {
		Temperature float32 `json:"temperature"` // 0 for embeddings
	} `json:"options"`
}

type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}
