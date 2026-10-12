// Package pztemplate loads the declarative game templates shipped with the
// application. Templates are data only: launch vectors, environment blocks and
// command strings are never executed or interpolated by this package.
package pztemplate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mj8724/gameserver/internal/domain"
)

const supportedSchema = "1.0"

var templateIDPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var branchPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type document struct {
	SchemaVersion string `yaml:"schema_version"`
	Metadata      struct {
		ID          string   `yaml:"id"`
		Name        string   `yaml:"name"`
		Category    string   `yaml:"category"`
		Icon        string   `yaml:"icon"`
		Author      string   `yaml:"author"`
		Version     string   `yaml:"version"`
		Description string   `yaml:"description"`
		SupportedOS []string `yaml:"supported_os"`
	} `yaml:"metadata"`
	Steam struct {
		AppID    any `yaml:"app_id"`
		Versions []struct {
			Label       string `yaml:"label"`
			Branch      string `yaml:"branch"`
			BuildID     string `yaml:"build_id"`
			Default     bool   `yaml:"default"`
			EvidenceRef string `yaml:"evidence_ref"`
		} `yaml:"versions"`
	} `yaml:"steam"`
	Ports []struct {
		Key         string `yaml:"key"`
		Label       string `yaml:"label"`
		Type        string `yaml:"type"`
		Default     int    `yaml:"default"`
		IsPrimary   bool   `yaml:"is_primary"`
		Description string `yaml:"description"`
	} `yaml:"ports"`
	Variables []struct {
		Key          string `yaml:"key"`
		Label        string `yaml:"label"`
		Type         string `yaml:"type"`
		Default      any    `yaml:"default"`
		Required     bool   `yaml:"required"`
		UserEditable bool   `yaml:"user_editable"`
		Description  string `yaml:"description"`
		Validation   struct {
			Min *float64 `yaml:"min"`
			Max *float64 `yaml:"max"`
		} `yaml:"validation"`
	} `yaml:"variables"`
}

// Catalog is a read-only set of validated templates.
type Catalog struct {
	templates map[domain.TemplateID]domain.Template
	order     []domain.TemplateID
	warnings  []string
}

// New loads every *.yaml template in dir. Files that fail validation are
// skipped and reported through Warnings instead of being partially applied.
func New(dir string) (*Catalog, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("template directory is required")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve template directory: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read template directory: %w", err)
	}
	catalog := &Catalog{templates: make(map[domain.TemplateID]domain.Template)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".yaml") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		template, err := loadFile(path)
		if err != nil {
			catalog.warnings = append(catalog.warnings, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		if _, exists := catalog.templates[template.Summary.ID]; exists {
			catalog.warnings = append(catalog.warnings, fmt.Sprintf("%s: duplicate template id %s", entry.Name(), template.Summary.ID))
			continue
		}
		catalog.templates[template.Summary.ID] = template
		catalog.order = append(catalog.order, template.Summary.ID)
	}
	sort.Slice(catalog.order, func(i, j int) bool { return catalog.order[i] < catalog.order[j] })
	return catalog, nil
}

// Warnings lists templates that were skipped during loading.
func (c *Catalog) Warnings() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.warnings...)
}

// List returns template summaries in stable id order.
func (c *Catalog) List() []domain.TemplateSummary {
	if c == nil {
		return nil
	}
	summaries := make([]domain.TemplateSummary, 0, len(c.order))
	for _, id := range c.order {
		summaries = append(summaries, c.templates[id].Summary)
	}
	return summaries
}

// Get returns one template by id.
func (c *Catalog) Get(id domain.TemplateID) (domain.Template, bool) {
	if c == nil {
		return domain.Template{}, false
	}
	template, ok := c.templates[id]
	return template, ok
}

func loadFile(path string) (domain.Template, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return domain.Template{}, err
	}
	var doc document
	decoder := yaml.NewDecoder(strings.NewReader(string(contents)))
	decoder.KnownFields(false)
	if err := decoder.Decode(&doc); err != nil {
		return domain.Template{}, fmt.Errorf("parse yaml: %w", err)
	}
	if doc.SchemaVersion != supportedSchema {
		return domain.Template{}, fmt.Errorf("unsupported schema_version %q", doc.SchemaVersion)
	}
	if !templateIDPattern.MatchString(doc.Metadata.ID) {
		return domain.Template{}, fmt.Errorf("invalid template id %q", doc.Metadata.ID)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return domain.Template{}, errors.New("metadata.name is required")
	}
	template := domain.Template{
		Summary: domain.TemplateSummary{
			ID:          domain.TemplateID(doc.Metadata.ID),
			Name:        doc.Metadata.Name,
			Category:    doc.Metadata.Category,
			Icon:        doc.Metadata.Icon,
			Author:      doc.Metadata.Author,
			Version:     doc.Metadata.Version,
			Description: doc.Metadata.Description,
			SupportedOS: append([]string(nil), doc.Metadata.SupportedOS...),
			AppID:       appIDString(doc.Steam.AppID),
		},
	}
	seenBranch := map[string]bool{}
	defaultSeen := false
	for _, version := range doc.Steam.Versions {
		branch := strings.TrimSpace(version.Branch)
		if strings.TrimSpace(version.Label) == "" || branch == "" || strings.TrimSpace(version.EvidenceRef) == "" {
			return domain.Template{}, errors.New("template versions need label, branch and evidence_ref")
		}
		if !branchPattern.MatchString(branch) {
			return domain.Template{}, fmt.Errorf("invalid template version branch %q", branch)
		}
		if seenBranch[branch] {
			return domain.Template{}, fmt.Errorf("duplicate template version branch %q", branch)
		}
		seenBranch[branch] = true
		if version.Default {
			if defaultSeen {
				return domain.Template{}, errors.New("only one template version may be the default")
			}
			defaultSeen = true
		}
		template.Versions = append(template.Versions, domain.TemplateVersion{
			Label: version.Label, Branch: branch, BuildID: version.BuildID,
			Default: version.Default, EvidenceRef: version.EvidenceRef,
		})
	}
	for _, variable := range doc.Variables {
		if strings.TrimSpace(variable.Key) == "" {
			return domain.Template{}, errors.New("variable without key")
		}
		if variable.Type == "" {
			return domain.Template{}, fmt.Errorf("variable %s without type", variable.Key)
		}
		template.Variables = append(template.Variables, domain.TemplateVariable{
			Key:          variable.Key,
			Label:        variable.Label,
			Description:  variable.Description,
			Type:         variable.Type,
			Default:      variable.Default,
			Required:     variable.Required,
			UserEditable: variable.UserEditable,
			Min:          variable.Validation.Min,
			Max:          variable.Validation.Max,
		})
	}
	for _, port := range doc.Ports {
		if strings.TrimSpace(port.Key) == "" {
			return domain.Template{}, errors.New("port without key")
		}
		if port.Default < 1 || port.Default > 65535 {
			return domain.Template{}, fmt.Errorf("port %s default %d out of range", port.Key, port.Default)
		}
		template.Ports = append(template.Ports, domain.TemplatePort{
			Key:         port.Key,
			Label:       port.Label,
			Protocol:    port.Type,
			Default:     port.Default,
			IsPrimary:   port.IsPrimary,
			Description: port.Description,
		})
	}
	return template, nil
}

func appIDString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case uint64:
		return fmt.Sprintf("%d", typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
