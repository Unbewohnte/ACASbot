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

import "math"

func SemanticSimilarity(a, b []float64) (float64, error) {
	// Use angular similarity instead of cosine for better semantic matching
	cosine, err := CosineSimilarity(a, b)
	if err != nil {
		return 0, err
	}

	// Convert to angular similarity
	return 1 - math.Acos(cosine)/math.Pi, nil
}
