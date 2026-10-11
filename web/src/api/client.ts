export class APIError extends Error { constructor(public status:number,public code:string,message:string){super(message)} }
export async function request<T>(path:string,options:RequestInit={}):Promise<T>{
 const headers=new Headers(options.headers)
 if(options.body)headers.set('Content-Type','application/json')
 if(options.method&&options.method!=='GET')headers.set('X-Requested-With','media-tool')
 const response=await fetch(`/api/v1${path}`,{...options,headers,credentials:'same-origin'})
 let body:{data?:T;error?:{code:string;message:string}}
 try{body=await response.json()}catch{throw new APIError(response.status,'invalid_response','后端未返回有效 JSON，请检查服务与代理配置')}
 if(response.status===401&&!path.startsWith('/auth/')&&typeof window!=='undefined')window.dispatchEvent(new Event('media-tool:session-expired'))
 if(!response.ok||body.error)throw new APIError(response.status,body.error?.code??'request_failed',body.error?.message??'请求失败')
 return body.data as T
}
export const post=<T>(path:string,body:unknown)=>request<T>(path,{method:'POST',body:JSON.stringify(body)})
