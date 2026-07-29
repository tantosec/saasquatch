// pkg/output/output_test.go
package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManager_WriteJSONReport(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "report.json")

	t.Run("Writes only found results to JSON", func(t *testing.T) {
		manager := NewManager(false, outputFile)

		manager.HandleResult(Result{Timestamp: time.Now(), Identifier: "identifier1", RuleName: "Rule 1", Service: "Service A", Found: true, URL: "http://found.com"})
		manager.HandleResult(Result{Timestamp: time.Now(), Identifier: "identifier2", RuleName: "Rule 2", Service: "Service B", Found: false, URL: "http://notfound.com"})
		manager.HandleResult(Result{Timestamp: time.Now(), Identifier: "identifier3", RuleName: "Rule 3", Service: "Service C", Found: true, URL: "http://found-again.com", Error: "some error"})

		err := manager.WriteJSONReport()
		if err != nil {
			t.Fatalf("WriteJSONReport failed: %v", err)
		}

		data, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		var results []outputResult
		if err := json.Unmarshal(data, &results); err != nil {
			t.Fatalf("Failed to unmarshal JSON report: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results in the JSON report, but got %d", len(results))
		}

		if results[0].Identifier != "identifier1" {
			t.Errorf("Expected first result identifier to be 'identifier1', but got '%s'", results[0].Identifier)
		}
		if results[1].Identifier != "identifier3" {
			t.Errorf("Expected second result identifier to be 'identifier3', but got '%s'", results[1].Identifier)
		}
	})

	t.Run("Does nothing if output file is not set", func(t *testing.T) {
		manager := NewManager(false, "") // No output file
		manager.HandleResult(Result{Found: true})
		err := manager.WriteJSONReport()
		if err != nil {
			t.Fatalf("Expected no error when output file is empty, but got %v", err)
		}
	})
}
