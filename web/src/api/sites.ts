export function siteFavicon(value?:string){return value&&/^\/api\/v1\/pt\/templates\/[a-z0-9-]{1,64}\/favicon(?:\?v=[A-Za-z0-9._-]+)?$/.test(value)?value:''}
