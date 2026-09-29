// Package archtest enforces the direct-import rules from ADR-001.
package archtest

import (
	"fmt"
	"strings"
)

const modulePrefix = "github.com/mj8724/gameserver"

type packageLayer string

const (
	layerDomain      packageLayer = "domain"
	layerPorts       packageLayer = "ports"
	layerApplication packageLayer = "application"
	layerAdapter     packageLayer = "adapter"
	layerCommand     packageLayer = "command"
	layerVersion     packageLayer = "version"
	layerArchtest    packageLayer = "archtest"
)

type packageInfo struct {
	layer       packageLayer
	adapterName string
	known       bool
}

func classify(importPath string) packageInfo {
	switch importPath {
	case modulePrefix + "/internal/domain":
		return packageInfo{layer: layerDomain, known: true}
	case modulePrefix + "/internal/ports":
		return packageInfo{layer: layerPorts, known: true}
	case modulePrefix + "/internal/application":
		return packageInfo{layer: layerApplication, known: true}
	case modulePrefix + "/internal/archtest":
		return packageInfo{layer: layerArchtest, known: true}
	case modulePrefix + "/internal/version":
		return packageInfo{layer: layerVersion, known: true}
	case modulePrefix + "/cmd/gameserver", modulePrefix + "/cmd/gs-lockhold":
		return packageInfo{layer: layerCommand, known: true}
	case modulePrefix + "/tools/pzoptions":
		return packageInfo{layer: layerCommand, known: true}
	case modulePrefix + "/tools/m2eval":
		// Acceptance tooling: a command-style package that reads test output.
		// It must stay free of project imports so the runner keeps working even
		// while the tree under test is mid-change.
		return packageInfo{layer: layerCommand, known: true}
	}

	const adaptersPrefix = modulePrefix + "/internal/adapters/"
	if strings.HasPrefix(importPath, adaptersPrefix) {
		name := strings.TrimPrefix(importPath, adaptersPrefix)
		if name != "" && !strings.Contains(name, "/") {
			return packageInfo{layer: layerAdapter, adapterName: name, known: true}
		}
	}
	if strings.HasPrefix(importPath, modulePrefix+"/internal/") || importPath == modulePrefix {
		return packageInfo{known: false}
	}
	return packageInfo{}
}

func normalizeImportPath(importPath string) string {
	if index := strings.Index(importPath, " ["); index >= 0 {
		return importPath[:index]
	}
	return importPath
}

func isStandardLibrary(importPath string, standard map[string]struct{}) bool {
	if standard != nil {
		_, ok := standard[importPath]
		return ok
	}
	first, _, _ := strings.Cut(importPath, "/")
	return first != "" && !strings.Contains(first, ".")
}

func forbiddenStandardImport(from packageInfo, importPath string) bool {
	switch from.layer {
	case layerDomain:
		switch importPath {
		case "os", "io", "io/fs", "path/filepath", "os/exec", "net", "net/http", "database/sql":
			return true
		}
	case layerApplication:
		switch importPath {
		case "os", "io/fs", "path/filepath", "os/exec", "net", "net/http", "database/sql":
			return true
		}
	case layerAdapter:
		if from.adapterName == "httpapi" {
			switch importPath {
			case "os", "io/fs", "path/filepath", "os/exec":
				return true
			}
		}
	}
	return false
}

func importViolation(fromPath, importedPath string, standard map[string]struct{}) error {
	fromPath = normalizeImportPath(fromPath)
	importedPath = normalizeImportPath(importedPath)
	from := classify(fromPath)
	if !from.known {
		return fmt.Errorf("unclassified project package %q", fromPath)
	}

	if !strings.HasPrefix(importedPath, modulePrefix) {
		if isStandardLibrary(importedPath, standard) {
			if forbiddenStandardImport(from, importedPath) {
				return fmt.Errorf("forbidden standard-library dependency: %s (%s) -> %s", fromPath, from.layer, importedPath)
			}
			return nil
		}
		if from.layer == layerDomain || from.layer == layerPorts || from.layer == layerApplication || from.layer == layerVersion || from.layer == layerArchtest {
			return fmt.Errorf("%s package %q may not import external dependency %q", from.layer, fromPath, importedPath)
		}
		return nil
	}

	to := classify(importedPath)
	if !to.known {
		return fmt.Errorf("import %q targets an unclassified package", importedPath)
	}

	allowed := false
	switch from.layer {
	case layerDomain:
		allowed = to.layer == layerDomain
	case layerPorts:
		allowed = to.layer == layerPorts || to.layer == layerDomain
	case layerApplication:
		allowed = to.layer == layerApplication || to.layer == layerPorts || to.layer == layerDomain
	case layerAdapter:
		allowed = to.layer == layerPorts || to.layer == layerDomain || (to.layer == layerAdapter && from.adapterName == to.adapterName)
		if from.adapterName == "httpapi" && to.layer == layerApplication {
			allowed = true
		}
	case layerCommand:
		allowed = true
	case layerVersion:
		allowed = to.layer == layerVersion
	case layerArchtest:
		allowed = to.layer == layerArchtest
	}
	if !allowed {
		return fmt.Errorf("forbidden import: %s (%s) -> %s (%s)", fromPath, from.layer, importedPath, to.layer)
	}
	return nil
}
