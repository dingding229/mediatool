import {describe,it,expect} from 'vitest'
import {canModify,canApprove,stepState,scanPercent} from './workbench'
import type {FileBatch,FileEntry} from './types'
describe('files/library proof-driven UI',()=>{
 it('never enables writes on a read-only or special file',()=>{const file={id:'owned',name:'Owned.mkv',path:'Owned.mkv',size:1,directory:false} satisfies FileEntry;expect(canModify(file)).toBe(true);expect(canModify({...file,unsupported:true})).toBe(false);expect(canModify({...file,capabilities:{read:true,write:false,move:false,recycle:false,readOnly:true,role:'download-source'}})).toBe(false)})
 it('requires unexpired conflict-free batch proof',()=>{const batch={state:'preview',expiresAt:new Date(Date.now()+60000).toISOString(),items:[{state:'preview',result:{}}]} as FileBatch;expect(canApprove(batch)).toBe(true);expect(canApprove({...batch,items:[{state:'blocked'}] as FileBatch['items']})).toBe(false);expect(canApprove({...batch,state:'running'})).toBe(false);expect(canApprove({...batch,expiresAt:new Date(0).toISOString()})).toBe(false)})
 it('only journaled steps count completed and unknown is not success',()=>{expect(stepState('needs_reconcile',1,0)).toBe('succeeded');expect(stepState('needs_reconcile',1,1)).toBe('needs_reconcile');expect(stepState('failed',1,2)).toBe('pending');expect(scanPercent(1,0)).toBe(0);expect(scanPercent(1,4)).toBe(25)})
})
