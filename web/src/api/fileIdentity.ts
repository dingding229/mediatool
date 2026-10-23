import {post} from './client';
export type CategoryChoice={name:string;directory:string};
export type FileIdentityResult={hint:Record<string,any>;matches:any[];selected?:any;classification?:CategoryChoice;categories:Record<string,CategoryChoice[]>;warning:string};
export function identifyFile(name:string,options:{query?:string;mediaType?:string;tmdbId?:number}={}){return post<FileIdentityResult>('/files/identify',{name,...options});}
