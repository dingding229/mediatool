import {request} from './client';
export interface ProxyFields {protocol:'http'|'socks5';host:string;port:number;username:string}
export interface ProxySettings extends Omit<ProxyFields,'protocol'> {protocol:ProxyFields['protocol']|'';tmdb:boolean;sites:boolean;revision:number;configured:boolean;hasPassword:boolean;needsInput:boolean}
export interface ProxyUpdate extends ProxyFields {tmdb:boolean;sites:boolean;revision:number;password:string;clearPassword:boolean;clear:boolean}
export const getNetworkProxy=()=>request<ProxySettings>('/settings/network-proxy');
export const saveNetworkProxy=(input:ProxyUpdate)=>request<ProxySettings>('/settings/network-proxy',{method:'PUT',body:JSON.stringify(input)});
