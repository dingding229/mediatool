import {request,type ListPage} from './client';
import type {MediaItem} from '../media/model';
export interface Category {id:string;name:string;group:string}
export interface CatalogPage extends ListPage<MediaItem> {categories?:Category[]}
export const managedDetail=(id:string)=>request<MediaItem>(`/media/library/managed/${encodeURIComponent(id)}`);
export function managedCatalog(q:string,parent:string,category:string,page:number){
 return request<CatalogPage>(`/media/library?source=managed&q=${encodeURIComponent(q)}&categoryId=${encodeURIComponent(category)}&page=${page}&pageSize=24&parentId=${encodeURIComponent(parent)}`);
}
