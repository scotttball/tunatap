package utils

import (
	"testing"
)

func TestParseOCID(t *testing.T) {
	tests := []struct {
		name       string
		ocid       string
		wantNil    bool
		wantType   string
		wantRealm  string
		wantRegion string
	}{
		// Commercial cloud (oc1)
		{
			name:       "valid cluster OCID",
			ocid:       "ocid1.cluster.oc1.us-ashburn-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc1",
			wantRegion: "us-ashburn-1",
		},
		{
			name:       "valid cluster OCID different region",
			ocid:       "ocid1.cluster.oc1.us-phoenix-1.aaaaaaaa5ijhlzaj7657v6o5pv4a6rbzt5vnb4sdzlyam5yhkcl665pyqk5q",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc1",
			wantRegion: "us-phoenix-1",
		},
		{
			name:       "valid bastion OCID",
			ocid:       "ocid1.bastion.oc1.us-phoenix-1.aaaaaaaa987654321",
			wantNil:    false,
			wantType:   "bastion",
			wantRealm:  "oc1",
			wantRegion: "us-phoenix-1",
		},

		// US Government cloud (oc2 - FedRAMP)
		{
			name:       "gov cloud oc2 cluster",
			ocid:       "ocid1.cluster.oc2.us-langley-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc2",
			wantRegion: "us-langley-1",
		},
		{
			name:       "gov cloud oc2 bastion",
			ocid:       "ocid1.bastion.oc2.us-luke-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "bastion",
			wantRealm:  "oc2",
			wantRegion: "us-luke-1",
		},

		// US DoD cloud (oc3)
		{
			name:       "dod cloud oc3 cluster",
			ocid:       "ocid1.cluster.oc3.us-gov-ashburn-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc3",
			wantRegion: "us-gov-ashburn-1",
		},

		// UK Government cloud (oc4)
		{
			name:       "uk gov cloud oc4 cluster",
			ocid:       "ocid1.cluster.oc4.uk-gov-london-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc4",
			wantRegion: "uk-gov-london-1",
		},

		// Dedicated regions (oc8)
		{
			name:       "dedicated region oc8 cluster",
			ocid:       "ocid1.cluster.oc8.ap-chiyoda-1.aaaaaaaa123456789",
			wantNil:    false,
			wantType:   "cluster",
			wantRealm:  "oc8",
			wantRegion: "ap-chiyoda-1",
		},

		// Error cases
		{
			name:    "empty OCID",
			ocid:    "",
			wantNil: true,
		},
		{
			name:    "invalid OCID - too few parts",
			ocid:    "ocid1.cluster.oc1",
			wantNil: true,
		},
		{
			name:    "invalid OCID - wrong prefix",
			ocid:    "ocid2.cluster.oc1.us-ashburn-1.aaa",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCID(tt.ocid)

			if tt.wantNil {
				if result != nil {
					t.Errorf("ParseOCID(%q) = %+v, want nil", tt.ocid, result)
				}
				return
			}

			if result == nil {
				t.Errorf("ParseOCID(%q) = nil, want non-nil", tt.ocid)
				return
			}

			if result.ResourceType != tt.wantType {
				t.Errorf("ParseOCID(%q).ResourceType = %q, want %q", tt.ocid, result.ResourceType, tt.wantType)
			}
			if result.Realm != tt.wantRealm {
				t.Errorf("ParseOCID(%q).Realm = %q, want %q", tt.ocid, result.Realm, tt.wantRealm)
			}
			if result.Region != tt.wantRegion {
				t.Errorf("ParseOCID(%q).Region = %q, want %q", tt.ocid, result.Region, tt.wantRegion)
			}
		})
	}
}

func TestExtractRegionFromOCID(t *testing.T) {
	tests := []struct {
		name       string
		ocid       string
		wantRegion string
	}{
		{
			name:       "commercial cluster",
			ocid:       "ocid1.cluster.oc1.us-ashburn-1.aaaaaaaa123",
			wantRegion: "us-ashburn-1",
		},
		{
			name:       "different region cluster",
			ocid:       "ocid1.cluster.oc1.eu-frankfurt-1.aaaaaaaa456",
			wantRegion: "eu-frankfurt-1",
		},
		{
			name:       "empty OCID",
			ocid:       "",
			wantRegion: "",
		},
		{
			name:       "invalid OCID",
			ocid:       "not-an-ocid",
			wantRegion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRegionFromOCID(tt.ocid)
			if result != tt.wantRegion {
				t.Errorf("ExtractRegionFromOCID(%q) = %q, want %q", tt.ocid, result, tt.wantRegion)
			}
		})
	}
}

func TestIsClusterOCID(t *testing.T) {
	tests := []struct {
		ocid string
		want bool
	}{
		{"ocid1.cluster.oc1.us-ashburn-1.aaa", true},
		{"ocid1.bastion.oc1.us-ashburn-1.aaa", false},
		{"ocid1.compartment.oc1.us-ashburn-1.aaa", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.ocid, func(t *testing.T) {
			result := IsClusterOCID(tt.ocid)
			if result != tt.want {
				t.Errorf("IsClusterOCID(%q) = %v, want %v", tt.ocid, result, tt.want)
			}
		})
	}
}

func TestIsBastionOCID(t *testing.T) {
	tests := []struct {
		ocid string
		want bool
	}{
		{"ocid1.bastion.oc1.us-ashburn-1.aaa", true},
		{"ocid1.cluster.oc1.us-ashburn-1.aaa", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ocid, func(t *testing.T) {
			result := IsBastionOCID(tt.ocid)
			if result != tt.want {
				t.Errorf("IsBastionOCID(%q) = %v, want %v", tt.ocid, result, tt.want)
			}
		})
	}
}
