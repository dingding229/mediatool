import {request,post} from './client';
import type {Connection} from '../connections/model';
export interface ConnectionCheck {passed:boolean;state:string;message:string;reused?:boolean}
export const listConnections=()=>request<Connection[]>('/integrations');
export const checkConnection=(id:string)=>post<ConnectionCheck>(`/integrations/${encodeURIComponent(id)}/test`,{});
