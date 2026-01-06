package utils

import (
	"fmt"
	"strings"
)

// OCIDParts represents the parsed components of an OCI OCID.
// Format: ocid1.<resource_type>.<realm>.<region>.<unique_id>
type OCIDParts struct {
	Version      string // e.g., "ocid1"
	ResourceType string // e.g., "cluster", "bastion", "compartment"
	Realm        string // e.g., "oc1" (commercial), "oc2" (gov), "oc3" (gov-dod)
	Region       string // e.g., "us-ashburn-1", "us-luke-1"
	UniqueID     string // the unique identifier
}

// ParseOCID parses an OCI OCID into its component parts.
// Returns nil if the OCID format is invalid.
func ParseOCID(ocid string) *OCIDParts {
	if ocid == "" {
		return nil
	}

	parts := strings.SplitN(ocid, ".", 5)
	if len(parts) < 5 {
		return nil
	}

	// Validate it starts with "ocid1"
	if parts[0] != "ocid1" {
		return nil
	}

	return &OCIDParts{
		Version:      parts[0],
		ResourceType: parts[1],
		Realm:        parts[2],
		Region:       parts[3],
		UniqueID:     parts[4],
	}
}

// ExtractRegionFromOCID extracts the region from an OCID.
// Returns empty string if the OCID format is invalid.
func ExtractRegionFromOCID(ocid string) string {
	parts := ParseOCID(ocid)
	if parts == nil {
		return ""
	}
	return parts.Region
}

// ValidateOCID checks if an OCID is valid and optionally validates it matches expected type/region.
func ValidateOCID(ocid string, expectedType string, expectedRegion string) error {
	parts := ParseOCID(ocid)
	if parts == nil {
		return fmt.Errorf("invalid OCID format: %s", ocid)
	}

	if expectedType != "" && parts.ResourceType != expectedType {
		return fmt.Errorf("OCID is for '%s' but expected '%s'", parts.ResourceType, expectedType)
	}

	if expectedRegion != "" && parts.Region != expectedRegion {
		return fmt.Errorf("OCID is for region '%s' but config specifies '%s'", parts.Region, expectedRegion)
	}

	return nil
}

// IsClusterOCID checks if the OCID is for a cluster resource.
func IsClusterOCID(ocid string) bool {
	parts := ParseOCID(ocid)
	return parts != nil && parts.ResourceType == "cluster"
}

// IsBastionOCID checks if the OCID is for a bastion resource.
func IsBastionOCID(ocid string) bool {
	parts := ParseOCID(ocid)
	return parts != nil && parts.ResourceType == "bastion"
}

// GetRealmDisplayName returns a human-readable name for the realm.
func GetRealmDisplayName(realm string) string {
	switch realm {
	case "oc1":
		return "OCI Commercial"
	case "oc2":
		return "OCI Government"
	case "oc3":
		return "OCI Government (DoD)"
	case "oc4":
		return "OCI Government (UK)"
	case "oc8":
		return "OCI Dedicated"
	default:
		return fmt.Sprintf("OCI (%s)", realm)
	}
}
