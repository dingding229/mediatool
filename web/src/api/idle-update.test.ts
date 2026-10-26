import {it,expect,vi} from 'vitest';
import {request,post} from './client';
import {getIdleUpdate,saveIdleUpdate,checkIdleUpdate} from './updates';
vi.mock('./client',()=>({request:vi.fn().mockResolvedValue({}),post:vi.fn().mockResolvedValue({})}));
it('sends only backend writable DTO fields, never read-only reason or unknown state',async()=>{const value={enabled:true,startTime:'23:00',endTime:'05:00',quietMinutes:60,revision:2,reason:'只读状态'};await saveIdleUpdate(value,'fixture-only',true);const [path,options]=vi.mocked(request).mock.calls.at(-1)!;expect(path).toBe('/system/online-update/idle');expect(options?.method).toBe('PUT');expect(JSON.parse(options?.body as string)).toEqual({enabled:true,startTime:'23:00',endTime:'05:00',quietMinutes:60,revision:2,password:'fixture-only',confirm:true})});
it('keeps preview separate from execution and uses API transport',async()=>{await getIdleUpdate();await checkIdleUpdate();expect(request).toHaveBeenCalledWith('/system/online-update/idle');expect(post).toHaveBeenCalledWith('/system/online-update/idle/check',{})});
