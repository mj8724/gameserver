package pz

import (
	"errors"
	"strings"
	"testing"

	"github.com/mj8724/gameserver/internal/ports"
)

// stubCatalog implements ports.OptionCatalog without importing another adapter
// (adapters must not import each other, enforced by internal/archtest). The
// values mirror the generated repository catalogue for the entries used here;
// the real catalogue is validated in its own package.
type stubCatalog struct{ entries []ports.OptionSpec }

func (c stubCatalog) Options() []ports.OptionSpec { return c.entries }

func (c stubCatalog) Get(target ports.OptionTarget, name string) (ports.OptionSpec, bool) {
	for _, entry := range c.entries {
		if entry.Target == target && entry.Name() == name {
			return entry, true
		}
	}
	return ports.OptionSpec{}, false
}

func (c stubCatalog) Source() ports.CatalogSource { return ports.CatalogSource{BuildID: "stub"} }

func testCatalog() stubCatalog {
	return stubCatalog{entries: []ports.OptionSpec{
		{Target: ports.OptionTargetINI, Key: "AntiCheatChecksum", Type: "int", Writable: ports.OptionWritableRW},
		{Target: ports.OptionTargetINI, Key: "AllowCoop", Type: "bool", Writable: ports.OptionWritableRW},
		{Target: ports.OptionTargetINI, Key: "RCONPassword", Type: "string", Secret: true, Writable: ports.OptionWritableRW},
		{Target: ports.OptionTargetINI, Key: "Public", Type: "bool", Writable: ports.OptionWritableRO},
		{Target: ports.OptionTargetINI, Key: "MaxPlayers", Type: "int", Writable: ports.OptionWritableRO},
		{Target: ports.OptionTargetINI, Key: "Password", Type: "string", Secret: true, Writable: ports.OptionWritableRO},
		{Target: ports.OptionTargetINI, Key: "UDPPort", Type: "int", Writable: ports.OptionWritableRO},
		{Target: ports.OptionTargetSandboxVars, Path: "Zombies", Type: "int", Writable: ports.OptionWritableRW},
		{Target: ports.OptionTargetSandboxVars, Path: "Basement", Type: "string", Writable: ports.OptionWritableRW},
	}}
}

// OptionSpecForTest keeps the validation table terse.
type OptionSpecForTest struct {
	Name string
	Type string
	Enum []string
	Min  *float64
	Max  *float64
}

func portsSpec(in OptionSpecForTest) ports.OptionSpec {
	spec := ports.OptionSpec{Key: in.Name, Type: in.Type, Enum: in.Enum}
	if in.Min != nil {
		spec.Min = in.Min
	}
	if in.Max != nil {
		spec.Max = in.Max
	}
	return spec
}

func TestRouteOptionsSplitsByWriterOwnership(t *testing.T) {
	catalog := testCatalog()
	ini, sandbox, err := RouteOptions(catalog, map[string]string{
		"AntiCheatChecksum": "2",
		"Zombies":           "3",
		"AllowCoop":         "true",
		"Basement":          "Rural",
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(ini) == 0 {
		t.Fatalf("expected INI options, got %v", ini)
	}
	if len(sandbox) == 0 {
		t.Fatalf("expected sandbox options, got %v", sandbox)
	}
	// Variable-bound keys belong to the variables page and must be refused here.
	for _, name := range []string{"Public", "MaxPlayers", "Password", "UDPPort"} {
		_, _, err := RouteOptions(catalog, map[string]string{name: "true"})
		if !errors.Is(err, ErrOptionReadOnly) {
			t.Fatalf("%s must be read-only through the options path (err=%v)", name, err)
		}
	}
	if _, _, err := RouteOptions(catalog, map[string]string{"NotARealOption": "1"}); !errors.Is(err, ErrOptionUnknown) {
		t.Fatalf("unknown option must be rejected, got %v", err)
	}
}

func TestRouteOptionsSecretBlankKeepsCurrentValue(t *testing.T) {
	catalog := testCatalog()
	ini, sandbox, err := RouteOptions(catalog, map[string]string{"RCONPassword": ""})
	if err != nil {
		t.Fatalf("blank secret must be accepted as keep-current: %v", err)
	}
	if len(ini) != 0 || len(sandbox) != 0 {
		t.Fatalf("blank secret must not produce a write: ini=%v sandbox=%v", ini, sandbox)
	}
	ini, _, err = RouteOptions(catalog, map[string]string{"RCONPassword": "s3cret"})
	if err != nil || ini["RCONPassword"] != "s3cret" {
		t.Fatalf("secret value must route to the INI writer: %v %v", ini, err)
	}
}

func TestValidateOptionValueEnforcesTypesAndRanges(t *testing.T) {
	min, max := 0.0, 10.0
	cases := []struct {
		spec  OptionSpecForTest
		value string
		ok    bool
	}{
		{OptionSpecForTest{"Zombies", "int", nil, nil, nil}, "4", true},
		{OptionSpecForTest{"Zombies", "int", nil, nil, nil}, "4.5", false},
		{OptionSpecForTest{"Multiplier", "float", nil, &min, nil}, "0.5", true},
		{OptionSpecForTest{"Multiplier", "float", nil, nil, &max}, "11", false},
		{OptionSpecForTest{"Bool", "bool", nil, nil, nil}, "true", true},
		{OptionSpecForTest{"Bool", "bool", nil, nil, nil}, "yes", false},
		{OptionSpecForTest{"Enum", "enum", []string{"1", "2"}, nil, nil}, "2", true},
		{OptionSpecForTest{"Enum", "enum", []string{"1", "2"}, nil, nil}, "3", false},
		{OptionSpecForTest{"Text", "string", nil, nil, nil}, "hello world", true},
		{OptionSpecForTest{"Text", "string", nil, nil, nil}, "bad\nvalue", false},
	}
	for _, test := range cases {
		spec := portsSpec(test.spec)
		err := ValidateOptionValue(spec, test.value)
		if test.ok && err != nil {
			t.Fatalf("%s=%q should be valid: %v", test.spec.Name, test.value, err)
		}
		if !test.ok && err == nil {
			t.Fatalf("%s=%q should be rejected", test.spec.Name, test.value)
		}
	}
	if err := ValidateOptionValue(portsSpec(OptionSpecForTest{"Text", "string", nil, nil, nil}), "ok"); err != nil {
		t.Fatalf("plain text must pass: %v", err)
	}
	if !strings.Contains(OptionError{Name: "X", Kind: ErrOptionValue}.Error(), "X") {
		t.Fatal("OptionError must name the option")
	}
}
