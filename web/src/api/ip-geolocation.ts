import {request} from './client';
export interface IPLocation {ip:string;unavailable?:boolean;label?:string;country?:string;province?:string;city?:string;isp?:string}
export interface GeoStatus {configured:boolean;ipv4:boolean;ipv6:boolean;updatedAt:string|null;sha256:string;attribution:string}
const path='/ip-geolocation';
export const getGeoStatus=()=>request<GeoStatus>(path+'/settings');
export const saveGeoSettings=(downloadKey:string,decryptKey:string)=>request<GeoStatus>(path+'/settings',{method:'PUT',body:JSON.stringify({downloadKey,decryptKey})});
// Coalesce a rendered page into bounded administrator-only lookups. No external
// geolocation requests, and no display of unclassified historical private IPs.
const cache=new Map<string,{at:number;value:IPLocation}>();
let pending=new Map<string,Array<(v:IPLocation)=>void>>(),scheduled=false;
export function lookupIP(raw=''):Promise<IPLocation> {
 if(!raw)return Promise.resolve({ip:''});
 const hit=cache.get(raw);if(hit&&Date.now()-hit.at<60000)return Promise.resolve(hit.value);
 return new Promise(resolve=>{const group=pending.get(raw)||[];group.push(resolve);pending.set(raw,group);if(!scheduled){scheduled=true;queueMicrotask(flush)}});
}
async function flush(){
 const batch=pending;pending=new Map();scheduled=false;const keys=[...batch.keys()];
 for(let start=0;start<keys.length;start+=50){const slice=keys.slice(start,start+50);
 try{const values=await request<IPLocation[]>(path+'?'+new URLSearchParams({ips:slice.join(',')}));slice.forEach((key,i)=>{const value=values[i]||{ip:''};if(cache.size>=4096)cache.delete(cache.keys().next().value!);cache.set(key,{at:Date.now(),value});batch.get(key)?.forEach(done=>done(value))})}
 catch{slice.forEach(key=>batch.get(key)?.forEach(done=>done({ip:'',unavailable:true})))}
 }
}
