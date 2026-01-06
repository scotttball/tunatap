package bastion

import (
	"strings"
	"testing"
)

func TestGetTunnelCommand(t *testing.T) {
	cmd := GetTunnelCommand(
		"~/.ssh/id_rsa",
		6443,
		6443,
		"10.0.0.1",
		"ocid1.bastionsession.oc1.iad.test",
		"us-ashburn-1",
		"",
	)

	// Verify command contains expected parts
	if !strings.Contains(cmd, "ssh") {
		t.Error("Command should contain 'ssh'")
	}

	if !strings.Contains(cmd, "~/.ssh/id_rsa") {
		t.Error("Command should contain the private key path")
	}

	if !strings.Contains(cmd, "6443") {
		t.Error("Command should contain the port")
	}

	if !strings.Contains(cmd, "10.0.0.1") {
		t.Error("Command should contain the remote IP")
	}

	if !strings.Contains(cmd, "us-ashburn-1") {
		t.Error("Command should contain the region")
	}

	if !strings.Contains(cmd, "oraclecloud.com") {
		t.Error("Command should contain the bastion domain")
	}
}

func TestGetTunnelCommandWithSocksProxy(t *testing.T) {
	cmd := GetTunnelCommand(
		"~/.ssh/id_rsa",
		6443,
		6443,
		"10.0.0.1",
		"ocid1.bastionsession.oc1.iad.test",
		"us-ashburn-1",
		"localhost:1080",
	)

	if !strings.Contains(cmd, "ProxyCommand") {
		t.Error("Command with SOCKS proxy should contain ProxyCommand")
	}

	if !strings.Contains(cmd, "localhost:1080") {
		t.Error("Command should contain the SOCKS proxy address")
	}
}

func TestGetTunnelCommandGovCloud(t *testing.T) {
	// Use a session ID that indicates gov cloud (contains "2" in the realm)
	cmd := GetTunnelCommand(
		"~/.ssh/id_rsa",
		6443,
		6443,
		"10.0.0.1",
		"ocid1.bastionsession.oc2.iad.test", // oc2 = gov cloud
		"us-gov-ashburn-1",
		"",
	)

	if !strings.Contains(cmd, "oraclegovcloud.com") {
		t.Error("Gov cloud session should use oraclegovcloud.com domain")
	}
}

func TestGetInternalTunnelCommand(t *testing.T) {
	cmd := GetInternalTunnelCommand(
		6443,
		6443,
		"10.0.0.1",
		"ocid1.bastion.oc1.iad.test",
		"10.0.0.100",
		"us-ashburn-1",
		"ocid1.compartment.oc1..test",
		"ztb-internal.bastion.us-ashburn-1.oci.oracleiaas.com",
	)

	if !strings.Contains(cmd, "ssh") {
		t.Error("Command should contain 'ssh'")
	}

	if !strings.Contains(cmd, "ProxyCommand") {
		t.Error("Internal bastion command should contain ProxyCommand")
	}

	if !strings.Contains(cmd, "10.0.0.100") {
		t.Error("Command should contain the jumpbox IP")
	}

	if !strings.Contains(cmd, "opc@") {
		t.Error("Command should connect as opc user")
	}
}

func TestFormatLocalAddress(t *testing.T) {
	addr := FormatLocalAddress(6443)

	if addr != "localhost:6443" {
		t.Errorf("FormatLocalAddress(6443) = %q, want %q", addr, "localhost:6443")
	}
}

func TestFormatRemoteAddress(t *testing.T) {
	addr := FormatRemoteAddress("10.0.0.1", 6443)

	if addr != "10.0.0.1:6443" {
		t.Errorf("FormatRemoteAddress() = %q, want %q", addr, "10.0.0.1:6443")
	}
}

func TestFormatBastionAddress(t *testing.T) {
	addr := FormatBastionAddress("us-ashburn-1")

	expected := "host.bastion.us-ashburn-1.oci.oraclecloud.com:22"
	if addr != expected {
		t.Errorf("FormatBastionAddress() = %q, want %q", addr, expected)
	}
}

func TestFormatBastionGovAddress(t *testing.T) {
	addr := FormatBastionGovAddress("us-gov-ashburn-1")

	expected := "host.bastion.us-gov-ashburn-1.oci.oraclegovcloud.com:22"
	if addr != expected {
		t.Errorf("FormatBastionGovAddress() = %q, want %q", addr, expected)
	}
}

func TestGetBastionDomain(t *testing.T) {
	tests := []struct {
		bastionID  string
		wantDomain string
	}{
		{"ocid1.bastion.oc1.iad.test", "oraclecloud"},
		{"ocid1.bastion.oc2.iad.test", "oraclegovcloud"},
		{"ocid1.bastion.oc1.phx.test", "oraclecloud"},
	}

	for _, tt := range tests {
		t.Run(tt.bastionID, func(t *testing.T) {
			domain := GetBastionDomain(tt.bastionID)
			if domain != tt.wantDomain {
				t.Errorf("GetBastionDomain(%q) = %q, want %q", tt.bastionID, domain, tt.wantDomain)
			}
		})
	}
}

func TestGetBastionDomainAllRealms(t *testing.T) {
	tests := []struct {
		name       string
		bastionID  string
		wantDomain string
	}{
		// Commercial cloud
		{"commercial oc1", "ocid1.bastion.oc1.us-ashburn-1.aaaatest", "oraclecloud"},
		{"commercial oc1 phoenix", "ocid1.bastion.oc1.us-phoenix-1.aaaatest", "oraclecloud"},
		{"commercial oc1 frankfurt", "ocid1.bastion.oc1.eu-frankfurt-1.aaaatest", "oraclecloud"},

		// US Government cloud (FedRAMP)
		{"us gov oc2", "ocid1.bastion.oc2.us-langley-1.aaaatest", "oraclegovcloud"},
		{"us gov oc2 luke", "ocid1.bastion.oc2.us-luke-1.aaaatest", "oraclegovcloud"},

		// US DoD cloud
		{"us dod oc3", "ocid1.bastion.oc3.us-gov-ashburn-1.aaaatest", "oraclegovcloud"},
		{"us dod oc3 chicago", "ocid1.bastion.oc3.us-gov-chicago-1.aaaatest", "oraclegovcloud"},

		// UK Government cloud
		{"uk gov oc4", "ocid1.bastion.oc4.uk-gov-london-1.aaaatest", "oraclegovcloud"},
		{"uk gov oc4 cardiff", "ocid1.bastion.oc4.uk-gov-cardiff-1.aaaatest", "oraclegovcloud"},

		// Dedicated regions (oc8, oc9, etc.)
		{"dedicated oc8", "ocid1.bastion.oc8.ap-chiyoda-1.aaaatest", "oraclegovcloud"},
		{"dedicated oc9", "ocid1.bastion.oc9.me-dcc-doha-1.aaaatest", "oraclegovcloud"},
		{"dedicated oc10", "ocid1.bastion.oc10.ap-dcc-gazipur-1.aaaatest", "oraclegovcloud"},

		// Edge cases
		{"invalid short ocid", "ocid1.bastion", "oraclecloud"}, // Defaults to oc1
		{"empty ocid", "", "oraclecloud"},                      // Defaults to oc1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := GetBastionDomain(tt.bastionID)
			if domain != tt.wantDomain {
				t.Errorf("GetBastionDomain(%q) = %q, want %q", tt.bastionID, domain, tt.wantDomain)
			}
		})
	}
}

func TestExtractRealmFromOCID(t *testing.T) {
	tests := []struct {
		name      string
		ocid      string
		wantRealm string
	}{
		{"commercial oc1", "ocid1.bastion.oc1.us-ashburn-1.test", "oc1"},
		{"us gov oc2", "ocid1.bastion.oc2.us-langley-1.test", "oc2"},
		{"us dod oc3", "ocid1.bastion.oc3.us-gov-ashburn-1.test", "oc3"},
		{"uk gov oc4", "ocid1.bastion.oc4.uk-gov-london-1.test", "oc4"},
		{"dedicated oc8", "ocid1.bastion.oc8.ap-chiyoda-1.test", "oc8"},
		{"short ocid", "ocid1.bastion", "oc1"}, // Defaults
		{"very short", "ocid1", "oc1"},         // Defaults
		{"empty", "", "oc1"},                   // Defaults
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realm := extractRealmFromOCID(tt.ocid)
			if realm != tt.wantRealm {
				t.Errorf("extractRealmFromOCID(%q) = %q, want %q", tt.ocid, realm, tt.wantRealm)
			}
		})
	}
}

func TestGetDomainFromRealm(t *testing.T) {
	tests := []struct {
		realm      string
		wantDomain string
	}{
		{"oc1", "oraclecloud"},
		{"oc2", "oraclegovcloud"},
		{"oc3", "oraclegovcloud"},
		{"oc4", "oraclegovcloud"},
		{"oc5", "oraclegovcloud"},
		{"oc8", "oraclegovcloud"},
		{"oc9", "oraclegovcloud"},
		{"oc10", "oraclegovcloud"},
		{"oc19", "oraclegovcloud"},
		{"oc20", "oraclegovcloud"},
		{"unknown", "oraclegovcloud"}, // Non-oc1 defaults to gov
	}

	for _, tt := range tests {
		t.Run(tt.realm, func(t *testing.T) {
			domain := getDomainFromRealm(tt.realm)
			if domain != tt.wantDomain {
				t.Errorf("getDomainFromRealm(%q) = %q, want %q", tt.realm, domain, tt.wantDomain)
			}
		})
	}
}

func TestGetBastionHostAddress(t *testing.T) {
	tests := []struct {
		bastionID string
		region    string
		want      string
	}{
		{
			"ocid1.bastion.oc1.iad.test",
			"us-ashburn-1",
			"host.bastion.us-ashburn-1.oci.oraclecloud.com:22",
		},
		{
			"ocid1.bastion.oc2.iad.test",
			"us-gov-ashburn-1",
			"host.bastion.us-gov-ashburn-1.oci.oraclegovcloud.com:22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			addr := GetBastionHostAddress(tt.bastionID, tt.region)
			if addr != tt.want {
				t.Errorf("GetBastionHostAddress() = %q, want %q", addr, tt.want)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"6443", 6443, false},
		{"22", 22, false},
		{"1", 1, false},
		{"65535", 65535, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"65536", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePort(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePort(%q) should error", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParsePort(%q) error = %v", tt.input, err)
				}
				if got != tt.want {
					t.Errorf("ParsePort(%q) = %d, want %d", tt.input, got, tt.want)
				}
			}
		})
	}
}
