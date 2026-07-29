package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/tantosec/saasquatch/internal/logging"
	"github.com/tantosec/saasquatch/internal/util"
	"gopkg.in/yaml.v3"
)

const SupportedVersion = 1

// Rule represents the structure of a single rule definition.
type Rule struct {
	Name      string             `yaml:"name"`
	Service   string             `yaml:"service"`
	Tags      []string           `yaml:"tags"`
	KnownGood string             `yaml:"known_good"`
	Request   Request            `yaml:"request"`
	Detection map[string]Leaf    `yaml:"detection"`
	Condition string             `yaml:"condition"`
	Captures  map[string]Capture `yaml:"captures,omitempty"`

	// Populated during loading
	FilePath string      `yaml:"-"`
	Cond     *vm.Program `yaml:"-"`
}

// Capture extracts a value from the matched response. The regex runs over the
// whole serialized response (headers + body); group 1 is captured, or the whole
// match if the regex has no group. All:true returns every match instead of the first.
type Capture struct {
	Regex string `yaml:"regex"`
	All   bool   `yaml:"all,omitempty"`
}

// Request defines the HTTP request details for a rule.
type Request struct {
	Method   string `yaml:"method"`
	Path     string `yaml:"path"`           // Using a slice to support multiple paths if needed
	Port     int    `yaml:"port,omitempty"` // omitempty means it won't be in YAML if zero/default
	Protocol string `yaml:"protocol"`
}

// Leaf is a single named matcher: exactly one of status, body or header.
type Leaf struct {
	Status        []int               `yaml:"status,omitempty"`
	Body          []string            `yaml:"body,omitempty"`
	Header        map[string][]string `yaml:"header,omitempty"`
	CaseSensitive bool                `yaml:"caseSensitive,omitempty"`
}

func CompileCondition(condition string, detection map[string]Leaf) (*vm.Program, error) {
	env := make(map[string]any, len(detection))
	for name := range detection {
		env[name] = false
	}
	return expr.Compile(condition, expr.Env(env), expr.AsBool())
}

func (rule *Rule) validate(identifierPlaceholder string) error {
	// Check for mandatory top-level fields.
	if rule.Name == "" {
		return errors.New("missing mandatory field: name")
	}
	if rule.Service == "" {
		return errors.New("missing mandatory field: service")
	}
	if rule.KnownGood == "" {
		return errors.New("missing mandatory field: known_good")
	}
	if rule.Request.Path == "" {
		return errors.New("missing mandatory field: request.path")
	}

	// Check the request verb is reasonably sensable
	if !util.StringIsOnlyLetters(rule.Request.Method) {
		return errors.New("request.method can only contain letters")
	}

	switch strings.ToLower(rule.Request.Protocol) {
	case "http", "https":
	default:
		return fmt.Errorf("invalid request.protocol %q: must be 'http' or 'https'", rule.Request.Protocol)
	}

	// Check if the path is dynamic (contains the placeholder).
	placeholder := identifierPlaceholder
	if !strings.Contains(rule.Request.Path, placeholder) {
		return fmt.Errorf("static path (missing placeholder '%s')", placeholder)
	}

	if len(rule.Detection) == 0 {
		return errors.New("missing mandatory field: detection")
	}
	for name, leaf := range rule.Detection {
		if err := leaf.validate(placeholder); err != nil {
			return fmt.Errorf("detection %q: %w", name, err)
		}
	}

	if rule.Condition == "" {
		return errors.New("missing mandatory field: condition")
	}
	prog, err := CompileCondition(rule.Condition, rule.Detection)
	if err != nil {
		return fmt.Errorf("invalid condition %q: %w", rule.Condition, err)
	}
	rule.Cond = prog

	for name, capture := range rule.Captures {
		if err := capture.validate(identifierPlaceholder); err != nil {
			return fmt.Errorf("capture %q: %w", name, err)
		}
	}

	return nil
}

func (capture Capture) validate(identifierPlaceholder string) error {
	if capture.Regex == "" {
		return errors.New("missing mandatory field: regex")
	}
	return checkPattern(capture.Regex, identifierPlaceholder, true)
}

func (leaf Leaf) validate(identifierPlaceholder string) error {
	set := 0
	if len(leaf.Status) > 0 {
		set++
	}
	if len(leaf.Body) > 0 {
		set++
	}
	if len(leaf.Header) > 0 {
		set++
	}
	if set != 1 {
		return errors.New("must set exactly one of status, body or header")
	}

	for _, pattern := range leaf.Body {
		if err := checkPattern(pattern, identifierPlaceholder, leaf.CaseSensitive); err != nil {
			return err
		}
	}
	for name, patterns := range leaf.Header {
		if len(patterns) == 0 {
			return fmt.Errorf("header %q requires at least one pattern", name)
		}
		for _, pattern := range patterns {
			if err := checkPattern(pattern, identifierPlaceholder, leaf.CaseSensitive); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPattern(pattern, identifierPlaceholder string, caseSensitive bool) error {
	// We don't need the real identifier, just checking for valid syntax.
	p := strings.ReplaceAll(pattern, identifierPlaceholder, "dummy")
	if !caseSensitive {
		p = "(?i)" + p
	}
	if _, err := regexp.Compile(p); err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
	}
	return nil
}

func parseRuleFile(mapping *yaml.Node) (version int, rules *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		switch key.Value {
		case "version":
			value.Decode(&version)
		case "rules":
			if value.Kind == yaml.SequenceNode {
				rules = value
			}
		}
	}
	return version, rules
}

// loadAllRulesFromFiles reads all YAML files recursively from the given directory
func loadAllRulesFromFiles(rulesDir string, identifierPlaceholder string) ([]Rule, error) {
	logger := logging.GetLogger()
	allRules := make([]Rule, 0)
	logger.Debug("Loading rules from directory", "path", rulesDir)

	err := filepath.WalkDir(rulesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			logger.Error("Error walking directory", "path", path, "error", err)
			return nil
		}
		// Skip directories and non-YAML files
		if d.IsDir() || (!strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml")) {
			return nil
		}

		logger.Debug("Found rule file", "path", path)

		fileContent, err := os.ReadFile(path)
		if err != nil {
			logger.Error("Failed to read rule file", "path", path, "error", err)
			return nil
		}

		// Unmarshal the file into a generic yaml.Node (Avoid disgarding the whole file if one rule is bad)
		var rootNode yaml.Node
		err = yaml.Unmarshal(fileContent, &rootNode)
		if err != nil {
			logger.Error("Failed to parse YAML syntax from file", "path", path, "error", err)
			return nil
		}

		// Ensure the YAML is a mapping with a version and a rules list.
		if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 || rootNode.Content[0].Kind != yaml.MappingNode {
			logger.Error("YAML file is not a valid rule file (expected version + rules)", "path", path)
			return nil
		}

		version, ruleSequence := parseRuleFile(rootNode.Content[0])
		if ruleSequence == nil {
			logger.Error("YAML file has no 'rules' list", "path", path)
			return nil
		}
		if version != SupportedVersion {
			logger.Error("Unsupported rule file version", "path", path, "version", version, "supported", SupportedVersion)
			return nil
		}

		rulesLoadedFromFile := 0

		// Iterate through each node (rule) in the sequence and decode it individually
		for _, ruleNode := range ruleSequence.Content {
			var rule Rule

			err := ruleNode.Decode(&rule)
			if err != nil {
				// This catches type mismatches for a single rule
				logger.Warn("Skipping malformed rule entry in file", "path", path, "line", ruleNode.Line, "error", err)
				continue
			}

			// Application-level validation
			rule.FilePath = path // Set metadata before validation

			// Validate the fully decoded rule object.
			if err := rule.validate(identifierPlaceholder); err != nil {
				logger.Warn("Skipping invalid rule",
					"file", path,
					"line", ruleNode.Line,
					"ruleName", rule.Name,
					"error", err)
				continue
			}

			allRules = append(allRules, rule)
			rulesLoadedFromFile++

		}
		logger.Debug("Finished processing file", "path", path, "rules_loaded", rulesLoadedFromFile, "total_rules_so_far", len(allRules))
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking rules directory %q: %w", rulesDir, err)
	}

	logger.Debug("Finished loading all rules", "count", len(allRules))
	return allRules, nil
}

func LoadAllRulesMatchingTags(rulesDir string, tagsToMatch []string, identifierPlaceholder string) ([]Rule, error) {
	logger := logging.GetLogger()
	logger.Debug("Tags", "Looking for", tagsToMatch)

	allRules, err := loadAllRulesFromFiles(rulesDir, identifierPlaceholder)
	if err != nil {
		return nil, fmt.Errorf("couldn't get rules")
	}

	if len(tagsToMatch) == 0 {
		logger.Debug("No tags provided for filtering, returning all loaded rules.")
		return allRules, nil
	}

	rulesWithMatchingTags := make([]Rule, 0)

	for _, rule := range allRules {
		if util.HaveCommonElements(rule.Tags, tagsToMatch) {
			rulesWithMatchingTags = append(rulesWithMatchingTags, rule)
		} else {
			logger.Debug("⚠️ Skipping rule based on tags", "Name", rule.Name, "Tags", rule.Tags)
		}
	}

	return rulesWithMatchingTags, nil

}
