import {it,expect,vi}from 'vitest';
import{request,post}from './client';
import{listExternalKeys,revealExternalKey,deleteExternalKey}from './external-credentials';
vi.mock('./client',()=>({request:vi.fn(),post:vi.fn().mockResolvedValue({})}));
it('list adapter never retains secret fields and reveal only sends an action',async()=>{vi.mocked(request).mockResolvedValue([{id:'id',name:'fixture',scopes:[],token:'must-not-cache',token_ciphertext:'bad'}]);const v=await listExternalKeys();expect(JSON.stringify(v)).not.toContain('must-not-cache');await revealExternalKey('id','copy');expect(post).toHaveBeenCalledWith('/external-api/keys/id/reveal',{action:'copy'})});
it('delete only sends explicit confirmation',async()=>{await deleteExternalKey('id');expect(request).toHaveBeenCalledWith('/external-api/keys/id',{method:'DELETE',body:'{"confirm":true}'})});
