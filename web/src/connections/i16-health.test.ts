import {expect,it,vi,afterEach} from 'vitest';
import {readFileSync} from 'node:fs';
import {checkConnection,listConnections} from '../api/connections';
import {post,request} from '../api/client';
import {readinessLabel} from '../workflows/readiness';
vi.mock('../api/client',()=>({post:vi.fn(),request:vi.fn()}));
afterEach(()=>vi.clearAllMocks());
it('uses existing API routes, safely encodes IDs and retains reused receipt flag',async()=>{
 vi.mocked(post).mockResolvedValue({passed:true,state:'ready',message:'连接成功',reused:true});
 expect((await checkConnection('only/id')).reused).toBe(true);expect(post).toHaveBeenCalledWith('/integrations/only%2Fid/test',{});
 vi.mocked(request).mockResolvedValue([]);await listConnections();expect(request).toHaveBeenCalledWith('/integrations');
});
it('maps failures to a binary label and never globally changes workflow badge meanings',()=>{
 expect(readinessLabel({state:'unavailable',reason:'',nextAction:''})).toBe('连接失败');
 const component=readFileSync(new URL('./ConnectionHealth.vue',import.meta.url),'utf8');
 expect(component).toContain("state==='ready'?'连接成功':state==='disabled'?'已停用':'连接失败'");expect(component).toContain('最近检测');expect(component).toContain('尚无有效检测记录，已安排检测');
 const manager=readFileSync(new URL('./ConnectionManager.vue',import.meta.url),'utf8');expect(manager).toContain(':disabled="!row.enabled||!!testState"');
});
