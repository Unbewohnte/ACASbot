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

package db

import (
	"Unbewohnte/ACASbot/internal/domain"
	"Unbewohnte/ACASbot/internal/similarity"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_fk=true")
	if err != nil {
		return nil, err
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Инициализация таблицы
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS articles (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            content TEXT NOT NULL,
			title TEXT,
            embedding BLOB NOT NULL,
            source_url TEXT UNIQUE,
            created_at INTEGER NOT NULL,
			published_at INTEGER,
			citations INTEGER DEFAULT 0,
			original BOOLEAN DEFAULT 0,
			similar_urls TEXT DEFAULT '[]',
			affiliation TEXT,
			sentiment TEXT,
			justification TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_articles_time ON articles(created_at);
		CREATE INDEX IF NOT EXISTS idx_articles_original ON articles(original);
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_configs (
			user_id INTEGER PRIMARY KEY,
			vector_similarity_threshold REAL DEFAULT 0.5,
			days_lookback INTEGER DEFAULT 10,
			composite_vector_weight REAL DEFAULT 0.7,
			final_similarity_threshold REAL DEFAULT 0.65,
			xlsx_columns TEXT
		);`,
	)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) SaveArticle(article *domain.Article) error {
	embJSON, err := json.Marshal(article.Embedding)
	if err != nil {
		return err
	}

	similarJSON, err := json.Marshal(article.SimilarURLs)
	if err != nil {
		return err
	}

	_, err = db.Exec(`INSERT INTO articles(
        content, title, embedding, source_url, 
        created_at, published_at, citations, original, similar_urls, 
        affiliation, sentiment, justification
    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		article.Content,
		article.Title,
		embJSON,
		article.SourceURL,
		article.CreatedAt,
		article.PublishedAt,
		article.Citations,
		article.Original,
		similarJSON,
		article.Affiliation,
		article.Sentiment,
		article.Justification,
	)
	return err
}

func (db *DB) FindSimilar(target []float64, threshold float64, maxAgeDays uint) ([]domain.Article, error) {
	// Normalize the target vector once
	similarity.NormalizeVector(target)

	rows, err := db.Query(`
        SELECT id, content, title, embedding, source_url, created_at, published_at, citations, original, similar_urls, affiliation, sentiment, justification
        FROM articles 
        WHERE created_at >= ? AND original >= 1
    `, time.Now().AddDate(0, 0, -int(maxAgeDays)).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Article
	for rows.Next() {
		var a domain.Article
		var embJSON, similarURLsJSON []byte

		if err := rows.Scan(
			&a.ID,
			&a.Content,
			&a.Title,
			&embJSON,
			&a.SourceURL,
			&a.CreatedAt,
			&a.PublishedAt,
			&a.Citations,
			&a.Original,
			&similarURLsJSON,
			&a.Affiliation,
			&a.Sentiment,
			&a.Justification,
		); err != nil {
			continue // Skip problematic rows but continue processing
		}

		var embedding []float64
		if err := json.Unmarshal(embJSON, &embedding); err != nil {
			continue
		}

		if err := json.Unmarshal(similarURLsJSON, &a.SimilarURLs); err != nil {
			continue
		}

		similarity.NormalizeVector(embedding)
		sim, err := similarity.SemanticSimilarity(target, embedding)
		if err != nil || sim < threshold || math.IsNaN(sim) {
			continue
		}

		a.Embedding = embedding
		a.Similarity = sim
		results = append(results, a)
	}

	return results, nil
}

func (db *DB) HasExactDuplicate(content string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM articles WHERE content = ?",
		content,
	).Scan(&count)

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) GetExactDuplicate(content string) (*domain.Article, error) {
	var article domain.Article
	var embJSON, similarURLsJSON []byte

	err := db.QueryRow(`
        SELECT id, content, title, embedding, source_url, created_at, published_at, citations, original, similar_urls, affiliation, sentiment, justification
        FROM articles 
        WHERE content = ?
        LIMIT 1`,
		content,
	).Scan(
		&article.ID,
		&article.Content,
		&article.Title,
		&embJSON,
		&article.SourceURL,
		&article.CreatedAt,
		&article.PublishedAt,
		&article.Citations,
		&article.Original,
		&similarURLsJSON,
		&article.Affiliation,
		&article.Sentiment,
		&article.Justification,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Unmarshal embedding if needed
	if len(embJSON) > 0 {
		if err := json.Unmarshal(embJSON, &article.Embedding); err != nil {
			return nil, err
		}
	}

	if len(similarURLsJSON) > 0 {
		if err := json.Unmarshal(similarURLsJSON, &article.SimilarURLs); err != nil {
			return nil, err
		}
	}

	return &article, nil
}

func (db *DB) GetUserConfig(userID int64) (*UserConfig, error) {
	config := &UserConfig{}
	var columnsJSON string

	err := db.QueryRow(`
        SELECT 
            user_id,
            vector_similarity_threshold,
            days_lookback,
            composite_vector_weight,
            final_similarity_threshold,
            xlsx_columns
        FROM user_configs
        WHERE user_id = ?`, userID).Scan(
		&config.UserID,
		&config.VectorSimilarityThreshold,
		&config.DaysLookback,
		&config.CompositeVectorWeight,
		&config.FinalSimilarityThreshold,
		&columnsJSON,
	)

	if err == sql.ErrNoRows {
		db.SaveUserConfig(DefaultUserConfig(userID))
		return DefaultUserConfig(userID), nil
	}
	if err != nil {
		return nil, err
	}

	// Десериализуем колонки
	if err := json.Unmarshal([]byte(columnsJSON), &config.XLSXColumns); err != nil {
		return nil, err
	}

	return config, nil
}

func (db *DB) SaveUserConfig(config *UserConfig) error {
	columnsJSON, err := json.Marshal(config.XLSXColumns)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
        REPLACE INTO user_configs (
            user_id,
            vector_similarity_threshold,
            days_lookback,
            composite_vector_weight,
            final_similarity_threshold,
            xlsx_columns
        ) VALUES (?, ?, ?, ?, ?, ?)`,
		config.UserID,
		config.VectorSimilarityThreshold,
		config.DaysLookback,
		config.CompositeVectorWeight,
		config.FinalSimilarityThreshold,
		columnsJSON,
	)
	return err
}

func (db *DB) DeleteAllArticles() error {
	_, err := db.Exec("DELETE FROM articles")
	return err
}

func (db *DB) IncrementCitation(articleID int64) error {
	_, err := db.Exec("UPDATE articles SET citations = citations + 1 WHERE id = ?", articleID)
	return err
}
func (db *DB) GetAllArticles() ([]domain.Article, error) {
	rows, err := db.Query(`
        SELECT 
            id, content, title, embedding, source_url, 
            created_at, published_at, citations, original, similar_urls, 
            affiliation, sentiment, justification
        FROM articles
        ORDER BY published_at ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []domain.Article
	for rows.Next() {
		var a domain.Article
		var embJSON, similarURLsJSON []byte

		if err := rows.Scan(
			&a.ID,
			&a.Content,
			&a.Title,
			&embJSON,
			&a.SourceURL,
			&a.CreatedAt,
			&a.PublishedAt,
			&a.Citations,
			&a.Original,
			&similarURLsJSON,
			&a.Affiliation,
			&a.Sentiment,
			&a.Justification,
		); err != nil {
			return nil, err
		}

		// Распаковываем embedding
		if len(embJSON) > 0 {
			if err := json.Unmarshal(embJSON, &a.Embedding); err != nil {
				return nil, err
			}
		}

		// Распаковываем similar_urls
		if len(similarURLsJSON) > 0 {
			if err := json.Unmarshal(similarURLsJSON, &a.SimilarURLs); err != nil {
				return nil, err
			}
		}

		articles = append(articles, a)
	}
	return articles, nil
}

func (db *DB) HasArticleByURL(url string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM articles WHERE source_url = ?",
		url,
	).Scan(&count)

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) AddSimilarURL(originalArticleID int64, citingArticleURL string) error {
	// Проверяем, что статья оригинальная
	var original bool
	err := db.QueryRow("SELECT original FROM articles WHERE id = ?", originalArticleID).Scan(&original)
	if err != nil {
		return err
	}

	if !original {
		return fmt.Errorf("статья с ID %d не является оригинальной", originalArticleID)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var similarJSON []byte
	err = tx.QueryRow("SELECT similar_urls FROM articles WHERE id = ?", originalArticleID).Scan(&similarJSON)
	if err != nil {
		return err
	}

	var urls []string
	if len(similarJSON) > 0 {
		if err := json.Unmarshal(similarJSON, &urls); err != nil {
			return err
		}
	}

	// Проверяем, есть ли уже такой URL
	for _, u := range urls {
		if u == citingArticleURL {
			return tx.Commit() // ничего не меняем
		}
	}

	urls = append(urls, citingArticleURL)
	newSimilarJSON, err := json.Marshal(urls)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE articles SET similar_urls = ? WHERE id = ?", newSimilarJSON, originalArticleID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
