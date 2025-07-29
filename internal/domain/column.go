package domain

type XLSXColumn struct {
	Name     string `json:"name" db:"name"`
	Field    string `json:"field" db:"field"`
	LLMQuery string `json:"llm_query" db:"llm_query"`
}
