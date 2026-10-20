import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { expect, it } from 'vitest'
import allowlist from '../guards/copywriting-allowlist.json'
const root = resolve(import.meta.dirname, '../..')
it('removes the old view rather than mounting or wrapping it', () => { expect(existsSync(resolve(root, 'src/views/SubscriptionsView.vue'))).toBe(false); const page = readFileSync(resolve(root, 'src/views/SubscriptionWorkspaceView.vue'), 'utf8'); expect(page).not.toContain('SubscriptionsView'); expect(allowlist).not.toHaveProperty('web/src/views/SubscriptionWorkspaceView.vue') })
it('covers every former endpoint in the new workspace and composer', () => { const code = ['useSubscriptionWorkspace.ts', 'SubscriptionComposer.vue'].map(f => readFileSync(resolve(root, 'src/subscriptions', f), 'utf8')).join('\n'); for (const literal of ['/subscriptions?view=', '/integrations', 'post(\'/subscriptions\'', '/subscriptions/${input.id}', '/subscriptions/${record.id}', '/subscriptions/${deleteTarget.value.id}', '/subscriptions/${id}/history', '/subscription-runs/${id}', '/subscriptions/${record.id}/evaluate', '/search/media?mediaType=', '/metadata/details?mediaType=']) expect(code).toContain(literal) })
