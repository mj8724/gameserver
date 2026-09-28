package application

import (
	"encoding/json"
	"testing"
)

func TestValidateConfigUpdateEnforcesDeclaredFieldsTypesAndRanges(t *testing.T) {
	snapshot := ConfigSnapshot{
		Ports:        map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
		AllowedPorts: []string{"SERVER_PORT", "DIRECT_PORT"},
		Fields: []ConfigField{
			{Key: "SERVER_NAME", Type: "string", UserEditable: true},
			{Key: "SERVER_PASSWORD", Type: "password", UserEditable: true},
			{Key: "ADMIN_PASSWORD", Type: "password", Required: true, UserEditable: true},
			{Key: "MAX_PLAYERS", Type: "number", Validation: map[string]any{"min": 1, "max": 64}, UserEditable: true},
			{Key: "PVP_ENABLED", Type: "boolean", UserEditable: true},
			{Key: "READ_ONLY", Type: "string", UserEditable: false},
		},
	}
	valid := ConfigUpdate{Variables: map[string]any{
		"SERVER_NAME": "server_1", "SERVER_PASSWORD": "", "ADMIN_PASSWORD": "secret",
		"MAX_PLAYERS": json.Number("64"), "PVP_ENABLED": true,
	}, Ports: map[string]int{"SERVER_PORT": 65535}}
	if err := ValidateConfigUpdate(snapshot, valid); err != nil {
		t.Fatalf("valid config update rejected: %v", err)
	}

	cases := []struct {
		name string
		edit func(*ConfigUpdate)
		want string
	}{
		{"unknown field", func(update *ConfigUpdate) { update.Variables["UNKNOWN"] = "x" }, "不允许修改配置项: UNKNOWN"},
		{"read only field", func(update *ConfigUpdate) { update.Variables["READ_ONLY"] = "x" }, "不允许修改配置项: READ_ONLY"},
		{"unsafe server name", func(update *ConfigUpdate) { update.Variables["SERVER_NAME"] = "../bad" }, "服务器名称仅允许 1-64 位字母、数字、下划线和连字符"},
		{"long string", func(update *ConfigUpdate) { update.Variables["SERVER_NAME"] = string(make([]byte, 257)) }, "SERVER_NAME 过长"},
		{"required password empty", func(update *ConfigUpdate) { update.Variables["ADMIN_PASSWORD"] = "" }, "ADMIN_PASSWORD 不能为空"},
		{"password too long", func(update *ConfigUpdate) { update.Variables["ADMIN_PASSWORD"] = string(make([]byte, 257)) }, "ADMIN_PASSWORD 必须是 256 字符以内的文本"},
		{"number boolean rejected", func(update *ConfigUpdate) { update.Variables["MAX_PLAYERS"] = true }, "MAX_PLAYERS 必须是整数"},
		{"number fraction rejected", func(update *ConfigUpdate) { update.Variables["MAX_PLAYERS"] = json.Number("2.5") }, "MAX_PLAYERS 必须是整数"},
		{"number above range", func(update *ConfigUpdate) { update.Variables["MAX_PLAYERS"] = 65 }, "MAX_PLAYERS 超出允许范围"},
		{"boolean strict", func(update *ConfigUpdate) { update.Variables["PVP_ENABLED"] = "true" }, "PVP_ENABLED 必须是布尔值"},
		{"unknown port", func(update *ConfigUpdate) { update.Ports = map[string]int{"BOGUS": 123} }, "不允许修改端口: BOGUS"},
		{"port out of range", func(update *ConfigUpdate) { update.Ports = map[string]int{"SERVER_PORT": 0} }, "SERVER_PORT 必须在 1-65535 之间"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			update := ConfigUpdate{Variables: map[string]any{}, Ports: map[string]int{}}
			tc.edit(&update)
			err := ValidateConfigUpdate(snapshot, update)
			useCase, ok := err.(*UseCaseError)
			if !ok || useCase.Code != CodeValidation || useCase.Message != tc.want {
				t.Fatalf("ValidateConfigUpdate() error = %#v, want validation %q", err, tc.want)
			}
		})
	}
}
