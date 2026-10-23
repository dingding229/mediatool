// Package clientinfo supplies non-authenticating request evidence.
package clientinfo

import (
	"media-tool/internal/platform/clientip"
	"net/http"
	"net/netip"
)

type Address = clientip.Address

func ValidateProxies(v []string) bool                   { return clientip.ValidateProxies(v) }
func PublicIP(v netip.Addr) bool                        { return clientip.PublicIP(v) }
func ClientAddress(r *http.Request, p []string) Address { return clientip.ClientAddress(r, p) }
