import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import Editor from './IPBlockEditor.vue';
import {listIPBlocks,checkIPBlock,deleteIPBlock} from '../api/ip-blocklist';
let fetch:ReturnType<typeof vi.fn>,views:ReturnType<typeof setupComponent>[]=[];
const state=()=>{const v=setupComponent(Editor);views.push(v);return v.state};
const page={items:[],total:0,revision:7,status:{available:true,retrySeconds:0,counterWarning:false},currentIp:'8.8.8.8',maxRules:10000};
beforeEach(()=>{fetch=vi.fn(async(path:string,o?:RequestInit)=>({ok:true,status:200,json:async()=>({data:path.includes('/check')?{matched:false,revision:7,candidateNetwork:'203.0.113.0/24',affectsCurrent:false}:o?.method?{revision:8}:page})}));vi.stubGlobal('fetch',fetch)});
afterEach(()=>{views.forEach(v=>v.unmount());views=[];vi.unstubAllGlobals()});
it('opens from an IP detail without mutation, previews read-only, then requires confirmation',async()=>{
 const s=state();await s.open('203.0.113.9');expect(s.form.revision).toBe(7);expect(s.confirm).toBe(false);expect(fetch.mock.calls.every(([,o])=>!o?.method)).toBe(true);
 await s.prepare();expect(s.confirm).toBe(true);expect(s.canonical).toBe('203.0.113.0/24');expect(fetch.mock.calls.at(-1)![0]).toContain('/check');expect(fetch.mock.calls.filter(([u])=>!u.includes('/check')).every(([,o])=>!o?.method)).toBe(true);
 await s.save();expect(s.visible).toBe(false);const write=fetch.mock.calls.at(-1)!;expect(write[1].method).toBe('POST');expect(JSON.parse(write[1].body)).toEqual({network:'203.0.113.0/24',note:'',revision:7,confirm:true,confirmSelf:false});
});
it('rejects missing addresses and oversized notes without submitting',async()=>{const s=state();await s.open();fetch.mockClear();await s.prepare();expect(s.error).toContain('请填写');expect(fetch).not.toHaveBeenCalled();s.form.network='203.0.113.9';s.form.note='字'.repeat(501);await s.prepare();expect(s.error).toContain('500');expect(fetch).not.toHaveBeenCalled()});
it('retains edits on conflict and never silently adopts a newer revision',async()=>{const s=state();await s.open('203.0.113.9');await s.prepare();s.form.note='手工备注';fetch.mockResolvedValueOnce({ok:false,status:409,json:async()=>({error:{code:'player_ip_conflict',message:'规则已变更'}})});await s.save();expect(s.form.note).toBe('手工备注');expect(s.form.revision).toBe(0);expect(s.visible).toBe(true);expect(s.error).toContain('重新读取')});
it('network-uncertain mutations prevent repeated submits until a list readback',async()=>{const s=state();await s.open('203.0.113.9');await s.prepare();fetch.mockRejectedValueOnce(Error('连接中断'));await s.save();expect(s.unknown).toBe(true);expect(s.error).toContain('勿重复提交');expect(s.visible).toBe(true)});
it('API layer keeps admin CSRF semantics and separates dry run from destructive routes',async()=>{await listIPBlocks(2,'中文 备注');expect(fetch.mock.calls.at(-1)![0]).toContain('page=2');await checkIPBlock('2001:db8::1');expect(JSON.parse(fetch.mock.calls.at(-1)![1].body)).toEqual({address:'2001:db8::1',network:''});await deleteIPBlock('owned-rule',7);expect(fetch.mock.calls.at(-1)![1].method).toBe('DELETE');expect(fetch.mock.calls.at(-1)![1].headers.get('X-Requested-With')).toBe('media-tool');await flush()});

it('does not overwrite an edited row using an automatically refreshed policy revision',async()=>{const s=state();await s.open('',{id:'owned-rule',network:'203.0.113.0/24',note:'旧草稿'},6);expect(s.form.revision).toBe(0);expect(s.error).toContain('规则已变更');expect(fetch.mock.calls.every(([,o])=>!o?.method)).toBe(true)});

it('previews a candidate with the editing ID, and prevents covered additions without a mutation',async()=>{
 const s=state();await s.open('192.0.2.9');fetch.mockResolvedValueOnce({ok:true,status:200,json:async()=>({data:{candidateNetwork:'192.0.2.0/24',coveredBy:'192.0.0.0/16',affectsCurrent:false}})});
 await s.prepare();expect(s.confirm).toBe(false);expect(s.error).toBe('已在 192.0.0.0/16 中');expect(fetch.mock.calls.every(([u,o])=>u.includes('/check')||!o?.method)).toBe(true);
 await s.open('',{id:'owned-rule',network:'192.0.2.0/24',note:'不变'});await s.prepare();expect(JSON.parse(fetch.mock.calls.at(-1)![1].body).excludeId).toBe('owned-rule');expect(s.confirm).toBe(true);
});
it('requires self-impact acknowledgement and preserves the 500 character note submission',async()=>{
 const s=state();await s.open('192.0.2.9');fetch.mockResolvedValueOnce({ok:true,status:200,json:async()=>({data:{candidateNetwork:'192.0.2.0/24',affectsCurrent:true}})});await s.prepare();expect(s.impact).toBe(true);expect(s.ack).toBe(false);s.form.note='字'.repeat(500);s.ack=true;await s.save();expect(JSON.parse(fetch.mock.calls.at(-1)![1].body).note).toHaveLength(500);
});
