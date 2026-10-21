const key='media-tool:pending-web-restore'
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/
export function pendingRestore():string{try{const id=sessionStorage.getItem(key)||'';return uuid.test(id)?id:''}catch{return ''}}
export function rememberRestore(id:string){if(uuid.test(id))try{sessionStorage.setItem(key,id)}catch{/* A restricted browser cannot persist the receipt locally. */}}
export function clearRestore(id:string){if(pendingRestore()===id)try{sessionStorage.removeItem(key)}catch{/* No authority or credential is stored here. */}}
