// M0/M1 contract types. OpenAPI source is api/openapi.yaml; generated clients are a
// follow-up task. Do not claim these types are generated.
export type Stage = 'planned' | 'partial' | 'foundation'
export interface ModuleInfo { id:string; name:string; summary:string; stage:Stage; milestone:string; features:string[] }
export interface Dependency { id:string; name:string; state:string; message:string }
export interface SystemStatus { name:string; version:string; environment:string; phase:string; dependencies:Dependency[]; publicUrl:string; strmEndpoint:string; allowedRoots:string[]; databaseConfigured:boolean; storageConfigured:boolean; proxyFallback:boolean; forceProxy:boolean; emby302Enabled:boolean }
export interface Job { id:string; kind:string; state:string; attempts:number; maxAttempts:number; createdAt:string; lastError?:string }
export interface StrmPreview { url:string; content:string; warning:string }

export interface CloudFile { id:string; name:string; path:string; size:number; isDirectory:boolean }
export interface StorageProbe { systemReady:boolean; loggedIn:boolean; pathReadable:boolean; file:CloudFile }
export interface StorageDiagnosis { file:CloudFile; playback:{directUrlReturned:boolean; mode:'redirect'|'proxy'|'unsupported'; expiresAt?:string; requiresUserAgent:boolean; requiresHeaders:boolean; targetValidated:boolean; mediaBytesChecked:boolean}; note:string }
