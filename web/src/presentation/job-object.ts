const list=(value:any):any[]=>Array.isArray(value)?value:Array.isArray(value?.items)?value.items:[];
// Resolve only existing labels. IDs remain grouping/navigation keys, never display fallbacks.
export function jobObject(row:any,accounts:any=[],objects:any=[]){
 const payload=row.payload&&typeof row.payload==='object'&&!Array.isArray(row.payload)?row.payload:{};
 const field=['accountId','integrationId','subscriptionId','assetId','downloadId','transferId','batchId','fileId'].find(k=>typeof payload[k]==='string'&&payload[k]);
 const object=field?payload[field]:row.kind==='system.check'?'system':row.id;
 const name=list(accounts).find(a=>a.id===object)?.name||list(objects).find(a=>a.id===object)?.title;
 return {field,object,name:typeof name==='string'&&name.trim()?name.trim():undefined};
}
