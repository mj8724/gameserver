// Package domain contains transport- and infrastructure-independent game server types.
package domain

// TemplateID identifies a declarative game template.
type TemplateID string

// TemplateSummary is the UI-facing template list projection. SupportedOS is
// compatibility metadata copied from the template file, not a support claim.
type TemplateSummary struct {
	ID          TemplateID
	Name        string
	Category    string
	Icon        string
	Author      string
	Version     string
	Description string
	SupportedOS []string
	AppID       string
}

// TemplateVariable is one declared configuration variable.
type TemplateVariable struct {
	Key          string
	Label        string
	Description  string
	Type         string
	Default      any
	Required     bool
	UserEditable bool
	Min          *float64
	Max          *float64
}

// TemplatePort is one declared port.
type TemplatePort struct {
	Key         string
	Label       string
	Protocol    string
	Default     int
	IsPrimary   bool
	Description string
}

// Template is a fully loaded game template.
type Template struct {
	Summary   TemplateSummary
	Variables []TemplateVariable
	Ports     []TemplatePort
}

// Variable returns the variable definition for key.
func (t Template) Variable(key string) (TemplateVariable, bool) {
	for _, variable := range t.Variables {
		if variable.Key == key {
			return variable, true
		}
	}
	return TemplateVariable{}, false
}

// Port returns the port definition for key.
func (t Template) Port(key string) (TemplatePort, bool) {
	for _, port := range t.Ports {
		if port.Key == key {
			return port, true
		}
	}
	return TemplatePort{}, false
}
