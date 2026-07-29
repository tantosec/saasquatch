// rules/rules_test.go
package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuleValidate provides exhaustive testing for the rule.validate() method.
// This function remains the same as it was already correct.
func TestRuleValidate(t *testing.T) {
	// A baseline valid rule to be modified by test cases.
	validRule := func() Rule {
		return Rule{
			Name:      "Valid Base Rule",
			Service:   "Test Service",
			KnownGood: "good-identifier",
			Request: Request{
				Method:   "GET",
				Path:     "example.com/IDENTIFIER",
				Protocol: "https",
			},
			Detection: map[string]Leaf{
				"match": {Status: []int{200}},
			},
			Condition: "match",
		}
	}

	testCases := []struct {
		name        string
		ruleFn      func() Rule // Use a function to get a fresh copy of the rule for each test.
		expectErr   bool
		errContains string
	}{
		// --- Valid Cases ---
		{
			name:      "Valid Status Rule",
			ruleFn:    validRule,
			expectErr: false,
		},
		{
			name: "Valid Regex Rule",
			ruleFn: func() Rule {
				r := validRule()
				r.Detection = map[string]Leaf{"match": {Body: []string{"some pattern"}}}
				return r
			},
			expectErr: false,
		},
		{
			name: "Condition references unknown leaf",
			ruleFn: func() Rule {
				r := validRule()
				r.Condition = "match or typo"
				return r
			},
			expectErr:   true,
			errContains: "invalid condition",
		},
		// ... (the rest of the TestRuleValidate cases from the previous answer can remain here) ...
		// --- Missing Mandatory Fields ---
		{
			name: "Missing Name",
			ruleFn: func() Rule {
				r := validRule()
				r.Name = ""
				return r
			},
			expectErr:   true,
			errContains: "missing mandatory field: name",
		},
		{
			name: "Missing Service",
			ruleFn: func() Rule {
				r := validRule()
				r.Service = ""
				return r
			},
			expectErr:   true,
			errContains: "missing mandatory field: service",
		},
		{
			name: "Missing KnownGood",
			ruleFn: func() Rule {
				r := validRule()
				r.KnownGood = ""
				return r
			},
			expectErr:   true,
			errContains: "missing mandatory field: known_good",
		},
		// --- Invalid Field Values & Logic ---
		{
			name: "Path Missing Placeholder",
			ruleFn: func() Rule {
				r := validRule()
				r.Request.Path = "example.com/static"
				return r
			},
			expectErr:   true,
			errContains: "static path (missing placeholder 'IDENTIFIER')",
		},
		{
			name: "Invalid HTTP Method with numbers",
			ruleFn: func() Rule {
				r := validRule()
				r.Request.Method = "GET1"
				return r
			},
			expectErr:   true,
			errContains: "request.method can only contain letters",
		},
		{
			name: "Valid Capture",
			ruleFn: func() Rule {
				r := validRule()
				r.Captures = map[string]Capture{"sso": {Regex: `Location: (https://\S+)`}}
				return r
			},
			expectErr: false,
		},
		{
			name: "Invalid Capture Regex",
			ruleFn: func() Rule {
				r := validRule()
				r.Captures = map[string]Capture{"broken": {Regex: `(unterminated`}}
				return r
			},
			expectErr:   true,
			errContains: "capture \"broken\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := tc.ruleFn()
			err := rule.validate("IDENTIFIER")

			if tc.expectErr {
				if err == nil {
					t.Fatal("Expected an error but got none")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("Expected error to contain '%s', but got: %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}
		})
	}
}

// createRuleFile is a helper to generate a YAML string for a rule.
func createRuleFile(name string, tags ...string) string {
	return `
version: 1
rules:
  - name: "` + name + `"
    service: "Service for ` + name + `"
    tags: ["` + strings.Join(tags, `", "`) + `"]
    known_good: "good"
    request:
      method: "GET"
      path: "IDENTIFIER.example.com"
      protocol: "https"
    detection:
      match: { status: [200] }
    condition: match
`
}

// TestLoadAllRulesMatchingTags tests the real file loading and tag filtering logic.
func TestLoadAllRulesMatchingTags(t *testing.T) {
	testCases := []struct {
		name          string
		setup         func(dir string) error // Function to create files for the test case
		tagsToMatch   []string
		expectedCount int
	}{
		{
			name: "No filter should return all rules",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "dev.yml"), []byte(createRuleFile("Dev Rule", "dev", "vcs")), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "crm.yml"), []byte(createRuleFile("CRM Rule", "crm")), 0644)
			},
			tagsToMatch:   []string{},
			expectedCount: 2,
		},
		{
			name: "Filter for a single tag that exists",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "dev.yml"), []byte(createRuleFile("Dev Rule", "dev", "vcs")), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "crm.yml"), []byte(createRuleFile("CRM Rule", "crm")), 0644)
			},
			tagsToMatch:   []string{"crm"},
			expectedCount: 1,
		},
		{
			name: "Filter for a tag that matches multiple files",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "dev1.yml"), []byte(createRuleFile("Dev Rule 1", "dev", "vcs")), 0644); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "dev2.yml"), []byte(createRuleFile("Dev Rule 2", "dev", "ci-cd")), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "other.yml"), []byte(createRuleFile("Other Rule", "hr")), 0644)
			},
			tagsToMatch:   []string{"dev"},
			expectedCount: 2,
		},
		{
			name: "Filter for multiple tags",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "dev.yml"), []byte(createRuleFile("Dev Rule", "dev", "vcs")), 0644); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "hr.yml"), []byte(createRuleFile("HR Rule", "hr")), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "crm.yml"), []byte(createRuleFile("CRM Rule", "crm")), 0644)
			},
			tagsToMatch:   []string{"vcs", "hr"},
			expectedCount: 2,
		},
		{
			name: "Filter for a nonexistent tag",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "dev.yml"), []byte(createRuleFile("Dev Rule", "dev")), 0644)
			},
			tagsToMatch:   []string{"nonexistent"},
			expectedCount: 0,
		},
		{
			name: "Filter should not match rules with no tags",
			setup: func(dir string) error {
				noTagsContent := strings.Replace(createRuleFile("No Tag Rule"), `    tags: [""]`, "", 1)
				return os.WriteFile(filepath.Join(dir, "notags.yml"), []byte(noTagsContent), 0644)
			},
			tagsToMatch:   []string{"dev"},
			expectedCount: 0,
		},
		{
			name: "No filter should still load rules with no tags",
			setup: func(dir string) error {
				noTagsContent := strings.Replace(createRuleFile("No Tag Rule"), `    tags: [""]`, "", 1)
				return os.WriteFile(filepath.Join(dir, "notags.yml"), []byte(noTagsContent), 0644)
			},
			tagsToMatch:   []string{},
			expectedCount: 1,
		},
		{
			name: "Directory with mixed valid and invalid rule files",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "good.yml"), []byte(createRuleFile("Good Rule", "dev")), 0644); err != nil {
					return err
				}
				// This file contains a rule that will fail validation and be skipped
				return os.WriteFile(filepath.Join(dir, "bad.yml"), []byte(createRuleFile("")), 0644)
			},
			tagsToMatch:   []string{"dev"},
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			// Use the setup function to create the specific file structure for this test case.
			if err := tc.setup(tempDir); err != nil {
				t.Fatalf("Test setup failed: %v", err)
			}

			// Run the function under test on our temporary directory
			filteredRules, err := LoadAllRulesMatchingTags(tempDir, tc.tagsToMatch, "IDENTIFIER")
			if err != nil {
				t.Fatalf("LoadAllRulesMatchingTags returned an unexpected error: %v", err)
			}

			if len(filteredRules) != tc.expectedCount {
				t.Errorf("Expected %d rules after filtering, but got %d", tc.expectedCount, len(filteredRules))
			}
		})
	}
}
