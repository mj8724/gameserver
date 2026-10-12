package optioncatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mj8724/gameserver/internal/ports"
)

const supportedSchema = "1.0"

var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type document struct {
	SchemaVersion string `yaml:"schema_version"`
	TemplateID    string `yaml:"template_id"`
	Source        struct {
		BuildID     string `yaml:"build_id"`
		ExtractedAt string `yaml:"extracted_at"`
		Command     string `yaml:"command"`
		Files       []struct {
			Path   string `yaml:"path"`
			SHA256 string `yaml:"sha256"`
		} `yaml:"files"`
	} `yaml:"source"`
	Options []struct {
		Target          string   `yaml:"target"`
		Key             string   `yaml:"key"`
		Path            string   `yaml:"path"`
		Label           string   `yaml:"label"`
		Type            string   `yaml:"type"`
		Secret          bool     `yaml:"secret"`
		Default         string   `yaml:"default"`
		Min             *float64 `yaml:"min"`
		Max             *float64 `yaml:"max"`
		Enum            []string `yaml:"enum"`
		Group           string   `yaml:"group"`
		Description     string   `yaml:"description"`
		RequiresRestart bool     `yaml:"requires_restart"`
		Writable        string   `yaml:"writable"`
		Clearable       bool     `yaml:"clearable"`
	} `yaml:"options"`
}

// Catalog is a validated option catalogue.
type Catalog struct {
	templateID string
	source     ports.CatalogSource
	options    []ports.OptionSpec
	byName     map[string]ports.OptionSpec
}

// New loads and validates <dir>/<templateID>.options.yaml.
func New(dir, templateID string) (*Catalog, error) {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(templateID) == "" {
		return nil, errors.New("option catalogue directory and template id are required")
	}
	path := filepath.Join(dir, templateID+".options.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("option catalogue: %w", err)
	}
	var doc document
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("option catalogue %s: %w", filepath.Base(path), err)
	}
	if doc.SchemaVersion != supportedSchema {
		return nil, fmt.Errorf("option catalogue schema %q is not supported", doc.SchemaVersion)
	}
	if doc.TemplateID != templateID {
		return nil, fmt.Errorf("option catalogue template id %q does not match %q", doc.TemplateID, templateID)
	}
	if strings.TrimSpace(doc.Source.BuildID) == "" || len(doc.Source.Files) == 0 {
		return nil, errors.New("option catalogue source fingerprint is required")
	}
	catalog := &Catalog{templateID: templateID, byName: map[string]ports.OptionSpec{}}
	catalog.source = ports.CatalogSource{
		BuildID: doc.Source.BuildID, ExtractedAt: doc.Source.ExtractedAt, Command: doc.Source.Command,
	}
	for _, file := range doc.Source.Files {
		if strings.TrimSpace(file.Path) == "" || len(file.SHA256) != 64 {
			return nil, fmt.Errorf("option catalogue source entry %q needs a path and a sha256", file.Path)
		}
		catalog.source.Files = append(catalog.source.Files, file.Path)
		catalog.source.SHA256 = append(catalog.source.SHA256, file.SHA256)
	}
	for index, entry := range doc.Options {
		spec, err := validate(entry.Target, entry.Key, entry.Path, entry.Label, entry.Type, entry.Secret,
			entry.Default, entry.Min, entry.Max, entry.Enum, entry.Group, entry.Description,
			entry.RequiresRestart, entry.Writable, entry.Clearable)
		if err != nil {
			return nil, fmt.Errorf("option catalogue entry %d: %w", index, err)
		}
		identity := string(spec.Target) + ":" + spec.Name()
		if _, exists := catalog.byName[identity]; exists {
			return nil, fmt.Errorf("option catalogue key %q is duplicated", identity)
		}
		catalog.byName[identity] = spec
		catalog.options = append(catalog.options, spec)
	}
	sort.Slice(catalog.options, func(i, j int) bool {
		if catalog.options[i].Target != catalog.options[j].Target {
			return catalog.options[i].Target < catalog.options[j].Target
		}
		return catalog.options[i].Name() < catalog.options[j].Name()
	})
	return catalog, nil
}

func validate(target, key, path, label, typ string, secret bool, def string, min, max *float64,
	enum []string, group, description string, restart bool, writable string, clearable bool) (ports.OptionSpec, error) {
	spec := ports.OptionSpec{
		Target: ports.OptionTarget(target), Key: key, Path: path, Label: label, Type: typ, Secret: secret,
		Default: def, Min: min, Max: max, Enum: enum, Group: group, Description: description,
		RequiresRestart: restart, Writable: writable, Clearable: clearable,
	}
	switch spec.Target {
	case ports.OptionTargetINI:
		if !namePattern.MatchString(key) || path != "" {
			return spec, errors.New("ini options need a key and no path")
		}
	case ports.OptionTargetSandboxVars:
		if !namePattern.MatchString(path) || key != "" {
			return spec, errors.New("sandboxvars options need a path and no key")
		}
	case ports.OptionTargetLaunch:
		if key == "" && path == "" {
			return spec, errors.New("launch options need a name")
		}
	default:
		return spec, fmt.Errorf("unknown option target %q", target)
	}
	switch typ {
	case "bool", "int", "float", "enum", "string":
	default:
		return spec, fmt.Errorf("unknown option type %q", typ)
	}
	if typ == "enum" && len(enum) == 0 {
		return spec, errors.New("enum options need at least one value")
	}
	if min != nil && max != nil && *min > *max {
		return spec, errors.New("option min must not exceed max")
	}
	switch writable {
	case ports.OptionWritableRW, ports.OptionWritableRO, ports.OptionWritableHidden:
	default:
		return spec, fmt.Errorf("unknown writable class %q", writable)
	}
	if secret && clearable {
		return spec, errors.New("secret options are not clearable")
	}
	return spec, nil
}

// Options returns every validated entry.
func (c *Catalog) Options() []ports.OptionSpec { return append([]ports.OptionSpec(nil), c.options...) }

// Get resolves one entry by target and physical name.
func (c *Catalog) Get(target ports.OptionTarget, name string) (ports.OptionSpec, bool) {
	spec, ok := c.byName[string(target)+":"+name]
	return spec, ok
}

// Source reports the catalogue provenance.
func (c *Catalog) Source() ports.CatalogSource { return c.source }
