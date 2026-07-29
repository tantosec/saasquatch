package util

import (
	"bufio"
	"errors"
	"hash/fnv"
	"math/rand"
	"os"
	"unicode"
)

func RandomLineFromFile(filePath string) (string, error) {

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(lines) == 0 {
		return "", errors.New("file is empty")
	}

	return lines[rand.Intn(len(lines))], nil
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandomString(n int) string {

	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func RandomStringWithSeed(n int, seed string) string {
	h := fnv.New64a()
	h.Write([]byte(seed))
	hashValue := h.Sum64()

	source := rand.NewSource(int64(hashValue))

	r := rand.New(source)

	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

func StringIsOnlyLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func HaveCommonElements(slice1, slice2 []string) bool {
	smallerSlice := slice1
	largerSlice := slice2
	if len(slice1) > len(slice2) {
		smallerSlice = slice2
		largerSlice = slice1
	}

	lookup := make(map[string]struct{}, len(smallerSlice))
	for _, value := range smallerSlice {
		lookup[value] = struct{}{}
	}

	for _, value := range largerSlice {
		if _, exists := lookup[value]; exists {
			return true
		}
	}

	return false
}
