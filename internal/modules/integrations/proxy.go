package integrations

import (
	"context"
	"media-tool/internal/modules/networkproxy"
	"net/http"
)

func (m *Manager) HTTPClient(ctx context.Context, scope string) (*http.Client, error) {
	if m.OutboundHTTP != nil {
		return m.OutboundHTTP, nil
	}
	r, e := (networkproxy.Repository{DB: m.Repository.DB, Vault: m.Repository.Vault}).Get(ctx)
	if e != nil {
		return nil, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proxyRevision != r.Revision {
		for _, client := range m.proxyClients {
			client.CloseIdleConnections()
		}
		m.proxyClients = nil
		m.proxyRevision = r.Revision
	}
	if client := m.proxyClients[scope]; client != nil {
		return client, nil
	}
	client, e := networkproxy.Client(r, scope)
	if e != nil {
		return nil, e
	}
	if m.proxyClients == nil {
		m.proxyClients = map[string]*http.Client{}
	}
	m.proxyClients[scope] = client
	return client, nil
}
