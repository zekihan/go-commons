package utils

import (
	"fmt"
	"io"
	"strings"
)

func DiffLines(w io.Writer, a, b string) {
	aLines := strings.Split(a, "\n")
	if len(aLines) == 1 && aLines[0] == "" {
		aLines = []string{}
	}

	bLines := strings.Split(b, "\n")
	if len(bLines) == 1 && bLines[0] == "" {
		bLines = []string{}
	}

	i, j := 0, 0
	lcs := longestCommonSubsequenceLines(aLines, bLines)

	for _, common := range lcs {
		for i < len(aLines) && aLines[i] != common {
			fmt.Fprintf(w, "- %s\n", aLines[i])
			i++
		}

		for j < len(bLines) && bLines[j] != common {
			fmt.Fprintf(w, "+ %s\n", bLines[j])
			j++
		}

		fmt.Fprintf(w, "  %s\n", common)

		i++
		j++
	}

	// Handle remaining lines.
	for i < len(aLines) {
		fmt.Fprintf(w, "- %s\n", aLines[i])
		i++
	}

	for j < len(bLines) {
		fmt.Fprintf(w, "+ %s\n", bLines[j])
		j++
	}
}

func longestCommonSubsequenceLines(sub1, sub2 []string) []string {
	sub1len, sub2len := len(sub1), len(sub2)

	dp := make([][]int, sub1len+1)
	for i := range dp {
		dp[i] = make([]int, sub2len+1)
	}

	// Fill table.
	for i := sub1len - 1; i >= 0; i-- {
		for j := sub2len - 1; j >= 0; j-- {
			switch {
			case sub1[i] == sub2[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] > dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Reconstruct LCS.
	var lcs []string

	i, j := 0, 0
	for i < sub1len && j < sub2len {
		switch {
		case sub1[i] == sub2[j]:
			lcs = append(lcs, sub1[i])
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}

	return lcs
}
