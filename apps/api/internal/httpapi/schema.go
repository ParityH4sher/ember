package httpapi

import (
	"fmt"
	"net/url"
	"reflect"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
)

type operation struct {
	OperationID  string         `json:"operationId"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
}

func convertOperations(source []plugins.Operation) []operation {
	result := make([]operation, len(source))
	for i, item := range source {
		result[i] = operation{
			OperationID: item.OperationID, Name: item.Name, Description: item.Description,
			Method: item.Method, Path: item.Path, InputSchema: item.InputSchema,
			OutputSchema: item.OutputSchema,
		}
	}
	return result
}

func properties(schema map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	raw, _ := schema["properties"].(map[string]any)
	for key, value := range raw {
		if property, ok := value.(map[string]any); ok {
			result[key] = property
		}
	}
	return result
}

func isWriteOnly(property map[string]any) bool {
	value, _ := property["writeOnly"].(bool)
	return value
}

func requiredFields(schema map[string]any) []any {
	required, _ := schema["required"].([]any)
	return required
}

func validateSchema(schema map[string]any, values map[string]any) error {
	props := properties(schema)
	for _, raw := range requiredFields(schema) {
		key, _ := raw.(string)
		if _, exists := values[key]; !exists {
			return fmt.Errorf("%s is required", key)
		}
	}
	additional, hasAdditional := schema["additionalProperties"].(bool)
	for key, value := range values {
		property, known := props[key]
		if !known {
			if hasAdditional && !additional {
				return fmt.Errorf("%s is not allowed", key)
			}
			continue
		}
		want, _ := property["type"].(string)
		valid := false
		switch want {
		case "string":
			var text string
			text, valid = value.(string)
			if valid && property["format"] == "uri" {
				parsed, err := url.Parse(text)
				valid = err == nil && parsed.IsAbs() &&
					(parsed.Scheme == "http" || parsed.Scheme == "https") &&
					parsed.Host != "" && parsed.User == nil
			}
		case "boolean":
			_, valid = value.(bool)
		case "number", "integer":
			number, ok := numberValue(value)
			valid = ok
			if valid && want == "integer" && number != float64(int64(number)) {
				valid = false
			}
			if valid {
				if minimum, ok := numberValue(property["minimum"]); ok && number < minimum {
					valid = false
				}
				if maximum, ok := numberValue(property["maximum"]); ok && number > maximum {
					valid = false
				}
			}
		case "object":
			object, ok := value.(map[string]any)
			valid = ok
			if valid {
				if err := validateSchema(property, object); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
			}
		case "array":
			_, valid = value.([]any)
		default:
			valid = true
		}
		if !valid {
			return fmt.Errorf("%s must be %s", key, want)
		}
		if enum, ok := property["enum"].([]any); ok {
			found := false
			for _, allowed := range enum {
				if reflect.DeepEqual(allowed, value) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s has an unsupported value", key)
			}
		}
	}
	return nil
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func nonNilMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}
