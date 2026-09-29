package pz

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Typed routing errors: the HTTP layer maps these to exact status codes and
// messages (plan D-C/D-D). They exist so no caller has to string-match.
var (
	ErrOptionUnknown  = errors.New("option is not part of the catalogue")
	ErrOptionReadOnly = errors.New("option is managed by another writer")
	ErrOptionHidden   = errors.New("option is not writable")
	ErrOptionValue    = errors.New("option value is invalid")
)

// OptionError carries the catalogue entry that failed validation.
type OptionError struct {
	Name   string
	Kind   error
	Detail string
}

func (e OptionError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %v", e.Name, e.Kind)
	}
	return fmt.Sprintf("%s: %v (%s)", e.Name, e.Kind, e.Detail)
}

func (e OptionError) Unwrap() error { return e.Kind }

// ValidateOptionValue checks a console-supplied value against the catalogue
// entry. Values arrive as text (the HTTP layer decodes JSON with json.Number),
// so integers/floats are parsed here rather than trusted.
func ValidateOptionValue(spec ports.OptionSpec, value string) error {
	if strings.ContainsAny(value, "\x00\n\r") {
		return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: "控制字符不被接受"}
	}
	switch spec.Type {
	case "bool":
		if value != "true" && value != "false" {
			return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: "需要 true 或 false"}
		}
	case "int":
		if _, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err != nil {
			return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: "需要整数"}
		}
	case "float":
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: "需要数值"}
		}
	case "enum":
		for _, candidate := range spec.Enum {
			if candidate == value {
				return nil
			}
		}
		return OptionError{Name: spec.Name(), Kind: ErrOptionValue,
			Detail: "取值范围: " + strings.Join(spec.Enum, ", ")}
	case "string":
		// any text without control characters
	default:
		return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: "未知类型 " + spec.Type}
	}
	if spec.Min != nil || spec.Max != nil {
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil
		}
		if spec.Min != nil && number < *spec.Min {
			return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: fmt.Sprintf("不得小于 %v", *spec.Min)}
		}
		if spec.Max != nil && number > *spec.Max {
			return OptionError{Name: spec.Name(), Kind: ErrOptionValue, Detail: fmt.Sprintf("不得大于 %v", *spec.Max)}
		}
	}
	return nil
}

// RouteOptions validates a console request against the catalogue and splits it
// per physical target. Exactly one writer owns a key: read-only and hidden
// entries are rejected here, so neither write path can steal another's key.
func RouteOptions(catalog ports.OptionCatalog, updates map[string]string) (ini map[string]string, sandbox map[string]string, err error) {
	if catalog == nil {
		return nil, nil, errors.New("option catalogue is unavailable")
	}
	if len(updates) == 0 {
		return nil, nil, nil
	}
	ini = map[string]string{}
	sandbox = map[string]string{}
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := updates[name]
		spec, ok := catalog.Get(ports.OptionTargetINI, name)
		target := ports.OptionTargetINI
		if !ok {
			spec, ok = catalog.Get(ports.OptionTargetSandboxVars, name)
			target = ports.OptionTargetSandboxVars
		}
		if !ok {
			return nil, nil, OptionError{Name: name, Kind: ErrOptionUnknown}
		}
		switch spec.Writable {
		case ports.OptionWritableRW:
		case ports.OptionWritableRO:
			return nil, nil, OptionError{Name: name, Kind: ErrOptionReadOnly, Detail: "由变量/模组页面管理"}
		case ports.OptionWritableHidden:
			return nil, nil, OptionError{Name: name, Kind: ErrOptionHidden}
		default:
			return nil, nil, OptionError{Name: name, Kind: ErrOptionHidden, Detail: "未知可写类别 " + spec.Writable}
		}
		// A secret left blank means "keep the current value" (never echo it).
		if spec.Secret && strings.TrimSpace(value) == "" {
			continue
		}
		if err := ValidateOptionValue(spec, value); err != nil {
			return nil, nil, err
		}
		switch target {
		case ports.OptionTargetINI:
			ini[spec.Key] = value
		default:
			sandbox[spec.Path] = value
		}
	}
	return ini, sandbox, nil
}

// ReadOptions returns the current values of the instance's INI and sandbox
// files, keyed by INI key / sandbox path. Values come from the files, never
// from cached state, so the console shows what the game will actually read.
func (c *Config) ReadOptions(ctx context.Context, id domain.InstanceID) (map[string]string, error) {
	values, err := c.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	name, err := c.serverName(ctx, id)
	if err != nil {
		return nil, err
	}
	path, err := SandboxVarsPath(c.dataRoot, string(id), name)
	if err != nil {
		return nil, err
	}
	sandbox, err := ReadSandbox(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, err
	}
	for key, value := range sandbox {
		if _, exists := values[key]; !exists {
			values[key] = value
		}
	}
	return values, nil
}
