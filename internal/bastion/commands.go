package bastion

import (
	"fmt"
	"strconv"
	"strings"
)

// extractRealmFromOCID safely extracts the realm (e.g., "oc1", "oc2") from an OCID.
// Returns "oc1" as default if the OCID format is invalid.
func extractRealmFromOCID(ocid string) string {
	parts := strings.Split(ocid, ".")
	if len(parts) < 3 {
		return "oc1" // Default to commercial cloud
	}
	return parts[2]
}

// getDomainFromRealm returns the OCI domain based on the realm identifier.
// Commercial cloud (oc1) uses oraclecloud.com, all other realms (oc2, oc3, oc4, etc.)
// use oraclegovcloud.com for government and specialized regions.
func getDomainFromRealm(realm string) string {
	if realm == "oc1" {
		return "oraclecloud"
	}
	// All other realms (oc2=US Gov, oc3=US DoD, oc4=UK Gov, oc8=dedicated, etc.)
	// use the government cloud domain
	return "oraclegovcloud"
}

// GetTunnelCommand generates the SSH command for connecting through a bastion.
func GetTunnelCommand(privateKeyFile string, localPort, remotePort int, remoteIP, sessionID, region, socksProxy string) string {
	realm := extractRealmFromOCID(sessionID)
	domain := getDomainFromRealm(realm)

	bastionHost := fmt.Sprintf("host.bastion.%s.oci.%s.com", region, domain)

	cmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new -o ProxyUseFdpass=no",
		privateKeyFile)

	if socksProxy != "" {
		cmd += fmt.Sprintf(" -o ProxyCommand='nc -X 5 -x %s %%h %%p'", socksProxy)
	}

	cmd += fmt.Sprintf(" -N -L %d:%s:%d -p 22 %s@%s",
		localPort, remoteIP, remotePort, sessionID, bastionHost)

	return cmd
}

// GetInternalTunnelCommand generates the SSH command for internal bastion type.
func GetInternalTunnelCommand(localPort, remotePort int, remoteIP, bastionID, jumpBoxIP, region, compartmentID, bastionLB string) string {
	cmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=accept-new -o ProxyUseFdpass=no "+
		"-N -L %d:%s:%d "+
		"-o ProxyCommand=\"ssh -o StrictHostKeyChecking=accept-new -W %%h:%%p -p 22 %s@%s\" "+
		"opc@%s",
		localPort, remoteIP, remotePort,
		bastionID, bastionLB,
		jumpBoxIP)

	return cmd
}

// FormatLocalAddress formats a local address for tunnel binding.
func FormatLocalAddress(port int) string {
	return fmt.Sprintf("localhost:%d", port)
}

// FormatRemoteAddress formats a remote address for tunnel destination.
func FormatRemoteAddress(ip string, port int) string {
	return fmt.Sprintf("%s:%d", ip, port)
}

// FormatBastionAddress formats the bastion service address.
func FormatBastionAddress(region string) string {
	return fmt.Sprintf("host.bastion.%s.oci.oraclecloud.com:22", region)
}

// FormatBastionGovAddress formats the bastion service address for gov cloud.
func FormatBastionGovAddress(region string) string {
	return fmt.Sprintf("host.bastion.%s.oci.oraclegovcloud.com:22", region)
}

// GetBastionDomain determines the domain based on the bastion ID.
func GetBastionDomain(bastionID string) string {
	realm := extractRealmFromOCID(bastionID)
	return getDomainFromRealm(realm)
}

// GetBastionHostAddress returns the full bastion host address.
func GetBastionHostAddress(bastionID, region string) string {
	domain := GetBastionDomain(bastionID)
	return fmt.Sprintf("host.bastion.%s.oci.%s.com:22", region, domain)
}

// ParsePort safely parses a port string to int.
func ParsePort(portStr string) (int, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port: %s", portStr)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}
	return port, nil
}
