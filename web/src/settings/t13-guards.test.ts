import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
const root=resolve(__dirname,'../../..'),read=(p:string)=>readFileSync(resolve(root,p),'utf8');
it('never exposes the retired execution flag or enable/disable commands in the new UI/API',()=>{for(const p of ['web/src/settings/OnlineUpdatePanel.vue','web/src/api/updates.ts'])expect(read(p)).not.toMatch(/executionEnabled|prepareControl|"enable"\|"disable"/);const api=JSON.parse(read('api/openapi.yaml'));const schemas=JSON.stringify(api.components.schemas);expect(schemas).not.toContain('executionEnabled');expect(api.paths['/api/v1/system/online-update/control'].post).toBeDefined()});
it('keeps real password/risk gates and only scopes migration deletion to the old settings key',()=>{const s=read('internal/platform/httpapi/online_update.go');expect(s).toContain('reauthenticateUpdate');expect(s).toContain('!in.Confirm || in.Password == ""');const migration=read('migrations/00087_update_settings.sql');expect(migration).toContain("DELETE FROM app_settings WHERE key='online_update.execution'");expect(migration).not.toMatch(/TRUNCATE|DELETE FROM (?!app_settings)|LIKE|DROP TABLE/)});
