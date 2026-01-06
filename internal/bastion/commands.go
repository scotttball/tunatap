package bastion

import (
	"fmt"
	"strconv"
	"strings"
)

// GetTunnelCommand generates the SSH command for connecting through a bastion.
func GetTunnelCommand(privateKeyFile string, localPort, remotePort int, remoteIP, sessionID, region, socksProxy string) string {
	oc := strings.Split(sessionID, ".")[2]
	domain := "oraclecloud"
	if strings.Contains(oc, "2") {
		domain = "oraclegovcloud"
	}

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
	oc := strings.Split(bastionID, ".")[2]
	if strings.Contains(oc, "2") {
		return "oraclegovcloud"
	}
	return "oraclecloud"
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
