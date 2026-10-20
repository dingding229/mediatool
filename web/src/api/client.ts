// Observe status without changing the data returned by request/post or adding serialized fields.
const responseStatuses=new WeakMap<object,number>()
export const responseStatus=(value:unknown)=>value!==null&&typeof value==='object'?responseStatuses.get(value):undefined
export interface FlowStatus {phase:string;retryable:boolean;nextAction:string;reason:string}
export class APIError extends Error { constructor(public status:number,public code:string,message:string,public requestId:string='',public flow?:FlowStatus){super(message)} }
export async function request<T>(path:string,options:RequestInit={}):Promise<T>{
 const headers=new Headers(options.headers)
 if(options.body&&!(options.body instanceof FormData))headers.set('Content-Type','application/json')
 if(options.method&&options.method!=='GET')headers.set('X-Requested-With','media-tool')
 const response=await fetch(`/api/v1${path}`,{...options,headers,credentials:'same-origin'})
 const requestId=response.headers?.get('X-Request-ID')??''
 let body:{data?:T;error?:{code:string;message:string;flow?:FlowStatus}}
 try{body=await response.json()}catch{throw new APIError(response.status,'invalid_response','后端未返回有效 JSON，请检查服务与代理配置',requestId)}
 if(response.status===401&&!path.startsWith('/auth/')&&typeof window!=='undefined')window.dispatchEvent(new Event('media-tool:session-expired'))
 if(!response.ok||body.error)throw new APIError(response.status,body.error?.code??'request_failed',body.error?.message??'请求失败',requestId,body.error?.flow)
 if(body.data!==null&&typeof body.data==='object')responseStatuses.set(body.data,response.status)
 return body.data as T
}
export const post=<T>(path:string,body:unknown)=>request<T>(path,{method:'POST',body:JSON.stringify(body)})
