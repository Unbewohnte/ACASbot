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

type UserConfig struct {
	UserID                    int64   `db:"user_id"`
	VectorSimilarityThreshold float64 `db:"vector_similarity_threshold"`
	DaysLookback              uint    `db:"days_lookback"`
	CompositeVectorWeight     float64 `db:"composite_vector_weight"`
	FinalSimilarityThreshold  float64 `db:"final_similarity_threshold"`
}

func DefaultUserConfig(userID int64) *UserConfig {
	return &UserConfig{
		UserID:                    userID,
		VectorSimilarityThreshold: 0.5,
		DaysLookback:              7,
		CompositeVectorWeight:     0.7,
		FinalSimilarityThreshold:  0.65,
	}
}
