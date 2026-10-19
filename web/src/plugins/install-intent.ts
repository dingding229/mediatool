// Only public definition identity and an opaque key are retained in this tab.
// Never persist configuration, credentials, actor identity or operation bodies.
const storageKey='media-tool:plugin-install-intents:v1'
type Journal=Record<string,string>
function read():Journal{
 try{const raw=sessionStorage.getItem(storageKey);if(!raw||raw.length>16384)return {}
 const data:unknown=JSON.parse(raw);if(!data||typeof data!=='object'||Array.isArray(data))return {}
 return Object.fromEntries(Object.entries(data).filter(([id,key])=>/^[a-z0-9-]{3,64}:[a-zA-Z0-9_-]{1,128}$/.test(id)&&typeof key==='string'&&/^[a-zA-Z0-9_-]{8,100}$/.test(key)).slice(0,32))
 }catch{return {}}
}
function write(value:Journal){try{sessionStorage.setItem(storageKey,JSON.stringify(value))}catch{/* Storage may be disabled; the view still retains the key until it is closed. */}}
export function installIntent(id:string,fingerprint:string,create:()=>string):string{
 const signature=id+':'+fingerprint,journal=read();if(journal[signature])return journal[signature]
 if(Object.keys(journal).length>=32)throw new Error('install_intent_limit')
 const key=create();journal[signature]=key;write(journal);return key
}
export function hasInstallIntent(id:string,fingerprint:string):boolean{return Boolean(read()[id+':'+fingerprint])}
export function completeInstallIntent(id:string,fingerprint:string){const journal=read();delete journal[id+':'+fingerprint];write(journal)}
