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

package article

type Article struct {
	ID             int64     `db:"id"`
	Title          string    `db:"title"`
	Content        string    `db:"content"`
	Embedding      []float64 `db:"embedding"`
	SourceURL      string    `db:"source_url"`
	CreatedAt      int64     `db:"created_at"`   // Unix timestamp
	PublishedAt    int64     `db:"published_at"` // Unix timestamp
	Citations      int64     `db:"citations"`
	SimilarURLs    []string  `db:"similar_urls"`
	Similarity     float64   `db:"-"`
	TrueSimilarity float64   `db:"-"`
	Affiliation    string    `db:"affiliation"`
	Sentiment      string    `db:"sentiment"`
	Justification  string    `db:"justification"`
	Errors         []error   `db:"-"`
}
