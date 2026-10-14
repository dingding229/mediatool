import { request, post } from './client'
export interface MediaGrant { libraryId: string; canPlay: boolean }
export interface MediaUser { id: string; username: string; displayName: string; enabled: boolean; mustChangePassword: boolean; revision: number; grants: MediaGrant[]; activeSessions: number; createdAt: string; updatedAt: string }
export interface MediaLibrary { id: string; name: string; mediaType: string; canPlay: boolean }
export interface MediaProvider { id: string; name: string; contract: string; runtime: string; stage: string; note: string; capabilities: Record<string, boolean> }
export interface MediaItem { id: string; title: string; year: string; mediaType: string; overview: string; poster: string; season?: number; episode?: number }
export const listMediaUsers = () => request<MediaUser[]>('/media-server/users')
export const listNativeLibraries = () => request<MediaLibrary[]>('/media-server/libraries')
export const memberLogin = (username: string, password: string) => post<{ authenticated: boolean; user: MediaUser }>('/member/auth/login', { username, password })
