package output

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/tantosec/saasquatch/internal/logging"
)

type Result struct {
	Timestamp  time.Time      `json:"timestamp"`
	Identifier string         `json:"identifier"`
	RuleName   string         `json:"rule_name"`
	Service    string         `json:"service"`
	Found      bool           `json:"found"`
	URL        string         `json:"url"`
	Error      string         `json:"error,omitempty"`
	Captures   map[string]any `json:"captures,omitempty"`
}

type outputResult struct {
	Timestamp  time.Time      `json:"timestamp"`
	Identifier string         `json:"identifier"`
	RuleName   string         `json:"rule_name"`
	Service    string         `json:"service"`
	URL        string         `json:"url"`
	Error      string         `json:"error,omitempty"`
	Captures   map[string]any `json:"captures,omitempty"`
}

type Manager struct {
	outputFile string
	jsonl      bool
	results    []Result
	mu         sync.Mutex
	colors     struct {
		success *color.Color
		info    *color.Color
	}
}

func filterFoundResults(results []Result) []outputResult {
	foundResults := make([]outputResult, 0)
	for _, result := range results {
		if result.Found {

			output := outputResult{
				Timestamp:  result.Timestamp,
				Identifier: result.Identifier,
				RuleName:   result.RuleName,
				Service:    result.Service,
				URL:        result.URL,
				Error:      result.Error,
				Captures:   result.Captures,
			}

			foundResults = append(foundResults, output)
		}
	}
	return foundResults
}

func formatCaptures(captures map[string]any) string {
	if len(captures) == 0 {
		return ""
	}
	names := make([]string, 0, len(captures))
	for name := range captures {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, len(names))
	for i, name := range names {
		switch v := captures[name].(type) {
		case []string:
			parts[i] = fmt.Sprintf("%s=%s", name, strings.Join(v, ","))
		default:
			parts[i] = fmt.Sprintf("%s=%v", name, v)
		}
	}
	return " | " + strings.Join(parts, " | ")
}

func NewManager(jsonl bool, outputFile string) *Manager {
	isTTY := isatty.IsTerminal(os.Stdout.Fd())
	if !isTTY || jsonl {
		color.NoColor = true
	}

	return &Manager{
		outputFile: outputFile,
		results:    make([]Result, 0),
		jsonl:      jsonl,
		colors: struct {
			success *color.Color
			info    *color.Color
		}{
			success: color.New(color.FgHiGreen, color.Bold),
			info:    color.New(color.FgCyan),
		},
	}
}

func (m *Manager) HandleResult(res Result) {
	logger := logging.GetLogger()

	m.mu.Lock()
	m.results = append(m.results, res)
	m.mu.Unlock()

	if res.Error != "" {
		return
	}

	if res.Found {

		output := outputResult{
			Timestamp:  res.Timestamp,
			Identifier: res.Identifier,
			RuleName:   res.RuleName,
			Service:    res.Service,
			URL:        res.URL,
			Error:      res.Error,
			Captures:   res.Captures,
		}

		if m.jsonl {
			jsonLine, err := json.Marshal(output)
			if err != nil {
				logger.Error("Failed to marshal result to JSON", "error", err)
			}
			fmt.Println(string(jsonLine))
		} else {
			m.colors.success.Print("[+] FOUND: ")
			fmt.Printf("%s | identifier '%s' | %s%s\n",
				m.colors.info.Sprint(res.RuleName),
				m.colors.info.Sprint(res.Identifier),
				res.URL,
				formatCaptures(res.Captures),
			)
		}
	}
}

type RunSummary struct {
	Total   int
	Found   int
	Errored int
}

func (m *Manager) Summary() RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()

	var s RunSummary
	for _, result := range m.results {
		s.Total++
		if result.Error != "" {
			s.Errored++
		} else if result.Found {
			s.Found++
		}
	}
	return s
}

func (m *Manager) WriteJSONReport() error {
	if m.outputFile == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	foundResults := filterFoundResults(m.results)
	data, err := json.MarshalIndent(foundResults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results to JSON: %w", err)
	}

	return os.WriteFile(m.outputFile, data, 0600)
}
