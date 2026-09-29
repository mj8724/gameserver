package application

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mj8724/gameserver/internal/ports"
)

// ValidateOptionUpdate enforces the catalogue rules for the options write path:
// the key must exist, its writable class must be rw, and the value must satisfy
// its declared type/range/enum. It returns a coded error whose message is the
// operator-facing detail.
func ValidateOptionUpdate(catalog ports.OptionCatalog, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if catalog == nil {
		return NewError(CodeOperationFailed, "选项目录不可用")
	}
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := updates[name]
		spec, ok := catalog.Get(ports.OptionTargetINI, name)
		if !ok {
			spec, ok = catalog.Get(ports.OptionTargetSandboxVars, name)
		}
		if !ok {
			return NewError(CodeOptionUnknown, fmt.Sprintf("未知配置项 %s", name))
		}
		switch spec.Writable {
		case ports.OptionWritableRW:
		case ports.OptionWritableRO:
			return NewError(CodeOptionReadOnly, fmt.Sprintf("%s 由变量/模组页面管理", name))
		default:
			return NewError(CodeOptionReadOnly, fmt.Sprintf("%s 不可写", name))
		}
		if spec.Secret && strings.TrimSpace(value) == "" {
			// "leave blank to keep the current value" is not a write.
			continue
		}
		if err := validateOptionValue(spec, value); err != nil {
			return NewError(CodeOptionValue, err.Error())
		}
	}
	return nil
}

func validateOptionValue(spec ports.OptionSpec, value string) error {
	if strings.ContainsAny(value, "\x00\n\r") {
		return fmt.Errorf("%s 含有非法字符", spec.Name())
	}
	switch spec.Type {
	case "bool":
		if value != "true" && value != "false" {
			return fmt.Errorf("%s 需要 true 或 false", spec.Name())
		}
	case "int":
		if _, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err != nil {
			return fmt.Errorf("%s 需要整数", spec.Name())
		}
	case "float":
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return fmt.Errorf("%s 需要数值", spec.Name())
		}
	case "enum":
		for _, candidate := range spec.Enum {
			if candidate == value {
				return nil
			}
		}
		return fmt.Errorf("%s 取值必须在 %s 之内", spec.Name(), strings.Join(spec.Enum, ", "))
	case "string":
	default:
		return fmt.Errorf("%s 的类型 %s 不受支持", spec.Name(), spec.Type)
	}
	if spec.Type == "int" || spec.Type == "float" {
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil
		}
		if spec.Min != nil && number < *spec.Min {
			return fmt.Errorf("%s 不得小于 %v", spec.Name(), *spec.Min)
		}
		if spec.Max != nil && number > *spec.Max {
			return fmt.Errorf("%s 不得大于 %v", spec.Name(), *spec.Max)
		}
	}
	return nil
}

// OptionRequiresRestart reports whether any accepted update needs a restart.
func OptionRequiresRestart(catalog ports.OptionCatalog, updates map[string]string) bool {
	if catalog == nil {
		return false
	}
	for name, value := range updates {
		spec, ok := catalog.Get(ports.OptionTargetINI, name)
		if !ok {
			spec, ok = catalog.Get(ports.OptionTargetSandboxVars, name)
		}
		if !ok || !spec.RequiresRestart {
			continue
		}
		if spec.Secret && strings.TrimSpace(value) == "" {
			continue
		}
		return true
	}
	return false
}

// ErrOptionWriteUnsupported is returned when the configured game adapter cannot
// accept option values at all.
var ErrOptionWriteUnsupported = errors.New("game adapter does not support option writes")
