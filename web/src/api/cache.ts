/** Read-only cache projections; absent fields in existing envelopes stay unknown. */
export interface TMDBConditions {operation:string;mediaType?:string;year?:string;language?:string;tmdbId?:number}
export interface CacheEntry {id:string;query:string;state:string;ttlSeconds:number;createdAt:string;bytes:number;count:number;conditions?:TMDBConditions}
