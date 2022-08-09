// Package random contains functions for some random data generation
package random

import "math/rand"

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") //nolint:gochecknoglobals // no need to reinitialize

// String function generates random string containing only (non)-capitalized latin letters
func String(n int) string {
	if n <= 0 {
		return ""
	}

	str := make([]rune, n)
	for i := range str {
		str[i] = letters[rand.Intn(len(letters))] //nolint:gosec // no need to be unique or secure here, favor to performance
	}

	return string(str)
}
