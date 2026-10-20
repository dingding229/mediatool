package outbox

// Shared by the plugin journal and the independent operational monitor. This
// bounds replay payloads, not the small durable source idempotency identities.
const PluginJournalLimit = 100000
const PluginJournalWarning = PluginJournalLimit * 8 / 10
