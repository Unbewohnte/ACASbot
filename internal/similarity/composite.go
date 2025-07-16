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

import "strings"

type CompositeSimilarity struct {
	VectorWeight float64 // 0.0-1.0
	TextWeight   float64
}

func NewCompositeSimilarity(vectorWeight float64) *CompositeSimilarity {
	return &CompositeSimilarity{
		VectorWeight: vectorWeight,
		TextWeight:   1.0 - vectorWeight,
	}
}

func (cs *CompositeSimilarity) Compare(text1, text2 string, vec1, vec2 []float64) (float64, error) {
	// Calculate vector similarity
	vecSim, err := CosineSimilarity(vec1, vec2)
	if err != nil {
		return 0, err
	}

	// Calculate text similarity
	textSim := CalculateEnhancedTextSimilarity(text1, text2)

	// Composite score
	return (vecSim*cs.VectorWeight + textSim*cs.TextWeight), nil
}

func CalculateEnhancedTextSimilarity(text1, text2 string) float64 {
	// 1. Remove proper nouns (town names, etc.)
	filtered1 := removeProperNouns(text1)
	filtered2 := removeProperNouns(text2)

	// 2. Calculate Jaccard similarity of important words
	return jaccardSimilarity(filtered1, filtered2)
}

func removeProperNouns(text string) string {
	// Implement using NLP library or heuristic rules
	// Example simple heuristic:
	words := strings.Fields(text)
	var filtered []string
	for _, word := range words {
		if !isLikelyProperNoun(word) {
			filtered = append(filtered, word)
		}
	}
	return strings.Join(filtered, " ")
}

func isLikelyProperNoun(word string) bool {
	// Simple heuristic - capitalize multi-letter words
	return len(word) > 3 && word[0] == strings.ToUpper(word[:1])[0]
}

func jaccardSimilarity(text1, text2 string) float64 {
	set1 := make(map[string]struct{})
	for _, word := range strings.Fields(text1) {
		if len(word) > 3 { // Ignore short words
			set1[strings.ToLower(word)] = struct{}{}
		}
	}

	set2 := make(map[string]struct{})
	for _, word := range strings.Fields(text2) {
		if len(word) > 3 {
			set2[strings.ToLower(word)] = struct{}{}
		}
	}

	intersection := 0
	for word := range set1 {
		if _, exists := set2[word]; exists {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
