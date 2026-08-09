// Package backendhealth defines the small HTTP readiness identity returned by
// catway and verified by a native local launcher before it trusts navigation.
package backendhealth

import (
	"fmt"

	"github.com/rohanthewiz/cats/internal/buildinfo"
)

const (
	Path    = "/.well-known/cats/health"
	Product = "cats"
)

type Report struct {
	Product string `json:"product"`
	Version string `json:"version"`
}

func Current() Report {
	return Report{Product: Product, Version: buildinfo.Version()}
}

func (r Report) Validate(expectedVersion string) error {
	if r.Product != Product {
		return fmt.Errorf("unexpected backend product %q", r.Product)
	}
	if r.Version == "" {
		return fmt.Errorf("backend version is empty")
	}
	if expectedVersion != "" && r.Version != expectedVersion {
		return fmt.Errorf("backend version %q does not match launcher %q", r.Version, expectedVersion)
	}
	return nil
}
