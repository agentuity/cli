package util

import (
	"time"

	"golang.org/x/exp/rand"
)

const alphaNumChars = "abcdefghjklmnpqrstuvwxyz23456789"

// RandStringBytes will generate a random string of length n using the alphaNumChars string.
func RandStringBytes(n int) string {
	b := make([]byte, n)
	l := len(alphaNumChars)
	for i := range b {
		b[i] = alphaNumChars[rand.Intn(l)]
	}
	return string(b)
}

func init() {
	rand.Seed(uint64(time.Now().UnixNano()))
}
