package playeraccess

import (
	"media-tool/internal/modules/playeraccess/clientinfo"
	"net/http"
	"net/netip"
)

// Compatibility wrappers preserve the existing gateway/admin API and rules.
type Address = clientinfo.Address

func ValidateProxies(values []string) bool { return clientinfo.ValidateProxies(values) }
func PublicIP(ip netip.Addr) bool          { return clientinfo.PublicIP(ip) }
func ClientAddress(r *http.Request, proxies []string) Address {
	return clientinfo.ClientAddress(r, proxies)
}
