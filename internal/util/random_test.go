package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandStringBytes(t *testing.T) {
	lengths := []int{0, 1, 5, 10, 20}

	for _, length := range lengths {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			result := RandStringBytes(length)

			assert.Equal(t, length, len(result))

			if length > 0 {
				for _, char := range result {
					assert.Contains(t, alphaNumChars, string(char))
				}
			}
		})
	}

	t.Run("uniqueness", func(t *testing.T) {
		const iterations = 10
		const length = 10
		results := make(map[string]bool)

		for i := 0; i < iterations; i++ {
			result := RandStringBytes(length)
			results[result] = true
		}

		assert.Equal(t, iterations, len(results))
	})
}
