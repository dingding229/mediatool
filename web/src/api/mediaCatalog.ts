import {request,type ListPage} from './client';
import type {MediaItem} from '../media/model';
export interface Category {id:string;name:string;group:string}
export interface CatalogPage extends ListPage<MediaItem> {categories?:Category[]}
export interface MediaAsset {
 id:string;revision:number;artifactsState:string;title:string|null;mediaType:string|null;
 path:string;state:string;revoked:boolean;fileScoped:boolean;poster:string;
 readonly createdAt:string;
}
export function listAssets(page:number,pageSize:number,assetId=''){
 return request<ListPage<MediaAsset>>(`/media/assets?page=${page}&pageSize=${pageSize}${assetId?'&assetId='+encodeURIComponent(assetId):''}`);
}
export const managedDetail=(id:string)=>request<MediaItem>(`/media/library/managed/${encodeURIComponent(id)}`);
export function managedCatalog(q:string,parent:string,category:string,page:number,pageSize=24){
 return request<CatalogPage>(`/media/library?source=managed&q=${encodeURIComponent(q)}&categoryId=${encodeURIComponent(category)}&page=${page}&pageSize=${pageSize}&parentId=${encodeURIComponent(parent)}`);
}
