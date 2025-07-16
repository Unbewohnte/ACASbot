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

package similarity

import (
	"fmt"
	"math"
	"strings"
)

func NormalizeVector(vec []float64) {
	magnitude := Magnitude(vec)
	if magnitude == 0 || math.IsNaN(magnitude) {
		return
	}
	for i := range vec {
		vec[i] /= magnitude
	}
}

func Magnitude(vec []float64) float64 {
	var sum float64
	for _, v := range vec {
		sum += v * v
	}
	return math.Sqrt(sum)
}

func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vectors must have same length")
	}

	var dotProduct float64
	for i := range a {
		dotProduct += a[i] * b[i]
	}

	// After normalization, magnitudes should be ~1.0
	// So we can just return the dot product directly
	return dotProduct, nil
}

// Optimized version for bulk comparisons
func BatchCosineSimilarity(target []float64, embeddings [][]float64) ([]float64, error) {
	results := make([]float64, len(embeddings))
	var err error

	for i, emb := range embeddings {
		results[i], err = CosineSimilarity(target, emb)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

func CalculateSimilarity(text1, text2 string) float64 {
	// Simple text similarity (can be enhanced)
	words1 := strings.Fields(text1)
	words2 := strings.Fields(text2)

	common := make(map[string]struct{})
	for _, word := range words1 {
		if len(word) > 3 { // Ignore short words
			common[strings.ToLower(word)] = struct{}{}
		}
	}

	var matches float64
	for _, word := range words2 {
		if len(word) > 3 {
			if _, exists := common[strings.ToLower(word)]; exists {
				matches++
			}
		}
	}

	return matches / float64(len(common))
}

// func NormalizeVector(vec []float64) {
// 	var sum float64
// 	for _, v := range vec {
// 		sum += v * v
// 	}
// 	magnitude := math.Sqrt(sum)

// 	// Avoid division by zero
// 	if magnitude == 0 {
// 		return
// 	}

// 	for i := range vec {
// 		vec[i] /= magnitude
// 	}
// }
