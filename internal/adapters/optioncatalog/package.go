// Package optioncatalog loads the declarative option catalogue generated from
// the vendor's own configuration files (tools/pzoptions). The catalogue decides
// which physical keys are writable, so loading is validated and fails closed:
// a malformed or tampered catalogue must never become a write policy.
package optioncatalog
