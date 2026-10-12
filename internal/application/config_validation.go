package application

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"unicode/utf8"

	"encoding/json"
)

var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateConfigUpdate enforces the legacy template-driven config rules at
// the application boundary. HTTP handlers may validate JSON shape, but must
// delegate editable-field/type/range policy here (or an equivalent use case).
func ValidateConfigUpdate(snapshot ConfigSnapshot, update ConfigUpdate) error {
	definitions := make(map[string]ConfigField, len(snapshot.Fields))
	for _, field := range snapshot.Fields {
		if field.Key != "" {
			definitions[field.Key] = field
		}
	}

	for key, value := range update.Variables {
		definition, ok := definitions[key]
		if !ok || !definition.UserEditable {
			return NewError(CodeValidation, fmt.Sprintf("不允许修改配置项: %s", key))
		}
		switch definition.Type {
		case "string":
			text, ok := value.(string)
			if !ok {
				return NewError(CodeValidation, fmt.Sprintf("%s 必须是文本", key))
			}
			if utf8.RuneCountInString(text) > 256 {
				return NewError(CodeValidation, fmt.Sprintf("%s 过长", key))
			}
			if key == "SERVER_NAME" && !serverNamePattern.MatchString(text) {
				return NewError(CodeValidation, "服务器名称仅允许 1-64 位字母、数字、下划线和连字符")
			}
		case "password":
			text, ok := value.(string)
			if !ok || utf8.RuneCountInString(text) > 256 {
				return NewError(CodeValidation, fmt.Sprintf("%s 必须是 256 字符以内的文本", key))
			}
			if definition.Required && text == "" {
				return NewError(CodeValidation, fmt.Sprintf("%s 不能为空", key))
			}
		case "number":
			number, ok := strictInteger(value)
			if !ok {
				return NewError(CodeValidation, fmt.Sprintf("%s 必须是整数", key))
			}
			if definition.Validation == nil {
				continue
			}
			minimum, hasMin := numericBound(definition.Validation["min"])
			maximum, hasMax := numericBound(definition.Validation["max"])
			if (hasMin && float64(number) < minimum) || (hasMax && float64(number) > maximum) {
				return NewError(CodeValidation, fmt.Sprintf("%s 超出允许范围", key))
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return NewError(CodeValidation, fmt.Sprintf("%s 必须是布尔值", key))
			}
		}
	}

	declaredPorts := make(map[string]struct{}, len(snapshot.AllowedPorts))
	for _, key := range snapshot.AllowedPorts {
		declaredPorts[key] = struct{}{}
	}
	// The existing port values are also the declared set for implementations
	// which do not need to provide a separate template catalog projection.
	for key := range snapshot.Ports {
		declaredPorts[key] = struct{}{}
	}
	for key, value := range update.Ports {
		if _, ok := declaredPorts[key]; !ok {
			return NewError(CodeValidation, fmt.Sprintf("不允许修改端口: %s", key))
		}
		if value < 1 || value > 65535 {
			return NewError(CodeValidation, fmt.Sprintf("%s 必须在 1-65535 之间", key))
		}
	}
	return nil
}

func strictInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil
	case int:
		return int64(number), true
	case int8:
		return int64(number), true
	case int16:
		return int64(number), true
	case int32:
		return int64(number), true
	case int64:
		return number, true
	case uint:
		if uint64(number) <= math.MaxInt64 {
			return int64(number), true
		}
	case uint8:
		return int64(number), true
	case uint16:
		return int64(number), true
	case uint32:
		return int64(number), true
	case uint64:
		if number <= math.MaxInt64 {
			return int64(number), true
		}
	}
	return 0, false
}

func numericBound(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case float32:
		return float64(number), true
	case float64:
		return number, true
	case string:
		parsed, err := strconv.ParseFloat(number, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
