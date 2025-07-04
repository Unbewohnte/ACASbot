package bot

import "sort"

// Левенштейн
func minDistance(a, b string) int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + min(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
			}
		}
	}
	return dp[m][n]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func (bot *Bot) findSimilarCommands(input string) []string {
	type cmdDistance struct {
		name     string
		distance int
	}

	var distances []cmdDistance
	for _, cmd := range bot.commands {
		dist := minDistance(input, cmd.Name)
		distances = append(distances, cmdDistance{cmd.Name, dist})
	}

	sort.Slice(distances, func(i, j int) bool {
		return distances[i].distance < distances[j].distance
	})

	var suggestions []string
	for i := 0; i < 3 && i < len(distances); i++ {
		suggestions = append(suggestions, distances[i].name)
	}

	return suggestions
}
