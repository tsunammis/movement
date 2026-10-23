let csrf='';
export class APIError extends Error {constructor(public code:string,message:string,public revision?:number){super(message)}}
export async function api<T>(path:string,options:RequestInit={}):Promise<T>{
 const headers=new Headers(options.headers);if(options.body)headers.set('Content-Type','application/json');if(csrf)headers.set('X-CSRF-Token',csrf);
 const response=await fetch('/api/v1'+path,{...options,headers,credentials:'same-origin'});
 const value=await response.json();if(!response.ok)throw new APIError(value.error?.code || 'http_error',value.error?.message || 'Request failed',value.error?.revision);return value as T;
}
export async function session(token?:string){const s=await api<{csrf_token:string}>('/session',token?{method:'POST',headers:{Authorization:'Bearer '+token}}:{});csrf=s.csrf_token}
export async function logout(){await api('/session',{method:'DELETE'});csrf=''}
const operations=new Map<string,string>();
export async function mutate<T>(path:string,method:string,data:unknown,revision?:number,override=false,claimToken?:string){
 const key=JSON.stringify([path,method,data,override]);let id=operations.get(key);if(!id){id=crypto.randomUUID();operations.set(key,id)}
 const result=await api<T>(path,{method,body:JSON.stringify({operation_id:id,actor:'local-user',expected_revision:revision,override,claim_token:claimToken||undefined,data})});operations.delete(key);return result;
}
