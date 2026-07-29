// pkg/util/utils_test.go
package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStringIsOnlyLetters(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"abc", true},
		{"ABC", true},
		{"HelloWorld", true},
		{"Hello World", false},
		{"Hello123", false},
		{"", true}, // An empty string contains no non-letters
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			if result := StringIsOnlyLetters(tc.input); result != tc.expected {
				t.Errorf("For input '%s', expected %v but got %v", tc.input, tc.expected, result)
			}
		})
	}
}

func TestHaveCommonElements(t *testing.T) {
	testCases := []struct {
		name     string
		slice1   []string
		slice2   []string
		expected bool
	}{
		{"No common elements", []string{"a", "b"}, []string{"c", "d"}, false},
		{"One common element", []string{"a", "b"}, []string{"b", "c"}, true},
		{"Multiple common elements", []string{"a", "b", "c"}, []string{"d", "c", "b"}, true},
		{"One slice is empty", []string{}, []string{"a", "b"}, false},
		{"Both slices are empty", []string{}, []string{}, false},
		{"Identical slices", []string{"x", "y"}, []string{"x", "y"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if result := HaveCommonElements(tc.slice1, tc.slice2); result != tc.expected {
				t.Errorf("Expected %v but got %v for slices %v and %v", tc.expected, result, tc.slice1, tc.slice2)
			}
		})
	}
}

func TestRandomStringWithSeed(t *testing.T) {
	// Test for determinism: the same seed should always produce the same string.
	str1 := RandomStringWithSeed(10, "my-test-seed")
	str2 := RandomStringWithSeed(10, "my-test-seed")
	if str1 != str2 {
		t.Errorf("Expected identical strings for the same seed, but got '%s' and '%s'", str1, str2)
	}

	// Test that a different seed produces a different string.
	str3 := RandomStringWithSeed(10, "a-different-seed")
	if str1 == str3 {
		t.Errorf("Expected different strings for different seeds, but got '%s' for both", str1)
	}

	// Test length.
	if len(str1) != 10 {
		t.Errorf("Expected string of length 10, but got %d", len(str1))
	}
}

func TestRandomLineFromFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	// Test with an empty file
	if _, err := os.Create(filePath); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	_, err := RandomLineFromFile(filePath)
	if err == nil {
		t.Error("Expected an error when reading from an empty file, but got none")
	}

	// Test with a file with content
	content := "line1\nline2\nline3"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	line, err := RandomLineFromFile(filePath)
	if err != nil {
		t.Errorf("Did not expect an error, but got %v", err)
	}
	if line != "line1" && line != "line2" && line != "line3" {
		t.Errorf("Expected one of the lines from the file, but got '%s'", line)
	}
}
