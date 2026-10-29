import {expect,it} from 'vitest';
import {jobObject} from './job-object';
it('resolves account and integration names from one existing collection',()=>{
 const accounts=[{id:'a',name:'HDSKY'}];
 expect(jobObject({kind:'site.metrics.refresh',payload:{accountId:'a'}},accounts)).toEqual({field:'accountId',object:'a',name:'HDSKY'});
 expect(jobObject({payload:{integrationId:'a'}},{items:accounts}).name).toBe('HDSKY');
});
it('preserves existing grouping-field precedence and transfer titles',()=>{
 expect(jobObject({payload:{accountId:'a',integrationId:'b'}},[{id:'b',name:'wrong'},{id:'a',name:'right'}]).name).toBe('right');
 expect(jobObject({payload:{transferId:'t'}},[],[{id:'t',title:'传输文件'}]).name).toBe('传输文件');
});
it('unknown IDs, malformed payload and empty names never leak IDs as labels',()=>{
 for(const payload of [null,[],{accountId:'unknown'},{accountId:17}])expect(jobObject({id:'job-id',payload},[{id:'a',name:'HDSKY'}]).name).toBeUndefined();
 expect(jobObject({payload:{accountId:'a'}},[{id:'a',name:'   '}]).name).toBeUndefined();expect(jobObject({id:'x',kind:'system.check'}).object).toBe('system');
});
