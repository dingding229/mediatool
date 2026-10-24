import {test,expect} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';

const WEEK=7*24*60*60;
function sql(query:string){
 if(process.env.E2E_ISOLATED!=='1'||process.env.E2E_BASE_URL!=='http://localhost:18084')throw Error('Use the isolated E2E runner');
 const dsn=process.env.TEST_DATABASE_URL||process.env.DATABASE_URL||'';
 if(new URL(dsn).pathname!=='/media_tool_test')throw Error('media_tool_test only');
 if(process.env.TEST_DB_EXEC_MODE==='psql')return execFileSync('psql',[dsn,'-XAtq','-v','ON_ERROR_STOP=1'],{input:query+'\n',encoding:'utf8'}).trim();
 return execFileSync('docker',['exec','-i',process.env.TEST_POSTGRES_CONTAINER||'media-tool-postgres-1','sh','-c','exec psql -XAtq -v ON_ERROR_STOP=1 -U "${POSTGRES_USER:-postgres}" -d media_tool_test'],{input:query+'\n',encoding:'utf8'}).trim();
}
test('T1 administrator cookie slides without rotation, expires idle, and obeys 30d cap',async({page})=>{
 const login=await page.request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username:process.env.E2E_ADMIN_USERNAME,password:process.env.E2E_ADMIN_PASSWORD}});
 expect(login.status()).toBe(200);
 const initial=(await page.context().cookies()).find(c=>c.name==='media_tool_session')!;
 expect(Boolean(initial)).toBe(true);
 const digest=createHash('sha256').update(initial.value).digest('hex');
 const state=()=>JSON.parse(sql(`SELECT json_build_object('created',extract(epoch from created_at),'expiry',extract(epoch from expires_at),'now',extract(epoch from statement_timestamp())) FROM administrator_sessions WHERE token_digest=decode('${digest}','hex')`));
 let data=state();expect(data.expiry-data.created).toBe(WEEK);
 const initialHeader=login.headers()['set-cookie']||'';
 expect(Number(initialHeader.match(/Max-Age=(\d+)/)?.[1])).toBe(WEEK);
 expect(Math.abs(initial.expires-Date.now()/1000-WEEK)).toBeLessThan(5);
 await page.goto('/dashboard');await expect(page.getByRole('heading',{name:'首页',exact:true})).toBeVisible();
 sql(`UPDATE administrator_sessions SET created_at=now()-interval '2 hours',expires_at=now()+interval '6 days 22 hours',last_active_at=now() WHERE token_digest=decode('${digest}','hex')`);
 const renewedResponse=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/auth/session');
 await page.reload();const renewed=await renewedResponse;expect(renewed.status()).toBe(200);
 // Browser Response.headers() omits cookie-related headers; inspect this
 // isolated test response through the complete header API instead.
 const renewal=(await renewed.headerValue('set-cookie'))||'';expect(Number(renewal.match(/Max-Age=(\d+)/)?.[1])).toBe(WEEK);
 data=state();expect(data.expiry-data.now).toBeGreaterThan(WEEK-5);
 const next=(await page.context().cookies()).find(c=>c.name==='media_tool_session')!;
 expect(next.value===initial.value).toBe(true);expect(Math.abs(next.expires-Date.now()/1000-WEEK)).toBeLessThan(5);
 const same=await page.request.get('/api/v1/auth/session');expect(same.status()).toBe(200);expect(Boolean(same.headers()['set-cookie'])).toBe(false);
 // Stop page polling before independently controlling the test-only SQL clock state.
 await page.goto('about:blank');
 sql(`UPDATE administrator_sessions SET created_at=now()-interval '29 days',expires_at=now()+interval '1 hour',last_active_at=now() WHERE token_digest=decode('${digest}','hex')`);
 const capped=await page.request.get('/api/v1/auth/session');expect(capped.status()).toBe(200);data=state();expect(data.expiry-data.created).toBe(30*24*60*60);
 const cappedAge=Number((capped.headers()['set-cookie']||'').match(/Max-Age=(\d+)/)?.[1]);expect(cappedAge).toBeGreaterThan(24*60*60-5);expect(cappedAge).toBeLessThanOrEqual(24*60*60);
 sql(`UPDATE administrator_sessions SET created_at=now()-interval '30 days',expires_at=now()+interval '7 days' WHERE token_digest=decode('${digest}','hex')`);
 const expired=await page.request.get('/api/v1/auth/session');expect(expired.status()).toBe(401);expect(Boolean(expired.headers()['set-cookie'])).toBe(false);
});
