import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import UsersPanel from './UsersPanel.vue';
import {readFileSync} from 'node:fs';
let view:ReturnType<typeof setupComponent>;
beforeEach(()=>vi.stubGlobal('fetch',vi.fn(async(url:string)=>({ok:true,status:200,json:async()=>({data:url.includes('/users?')?{items:[],total:0,page:1,pageSize:25}:[]})}))));
afterEach(()=>{view?.unmount();vi.unstubAllGlobals()});
it('keeps the original server-paged 25-row default and requests the selected size',async()=>{
 view=setupComponent(UsersPanel);await flush();expect(fetch).toHaveBeenCalledWith('/api/v1/media-server/users?page=1&pageSize=25',expect.anything());view.state.page=2;view.state.pageSize=10;await view.state.users.load();expect(fetch).toHaveBeenLastCalledWith('/api/v1/media-server/users?page=2&pageSize=10',expect.anything());
});
it('retains disabled, locked, expired and enabled account meanings',()=>{
 view=setupComponent(UsersPanel);const s=view.state;expect(s.status({enabled:false})).toBe('停用');expect(s.status({enabled:true,lockedUntil:'2099-01-01T00:00:00Z'})).toBe('锁定');expect(s.status({enabled:true,expiresAt:'2000-01-01T00:00:00Z'})).toBe('锁定');expect(s.statusHint({enabled:true,expiresAt:'2000-01-01T00:00:00Z'})).toBe('账号已到期');expect(s.status({enabled:true})).toBe('启用');
});
it('keeps all three accessible row actions and existing editor/detail ownership',()=>{
 const source=readFileSync(new URL('./UsersPanel.vue',import.meta.url),'utf8');for(const label of ['编辑用户','用户详情','更多'])expect(source).toContain(`aria-label="${label}"`);expect(source).toContain('role="list" aria-label="媒体用户列表"');expect(source).toContain('role="listitem"');expect(source).toContain('editor?.open(user,true)');expect(source).not.toContain('maxParentalRating');expect(source).not.toContain('media-user-grid');expect(source).toContain('@current-change="users.load"');
});
