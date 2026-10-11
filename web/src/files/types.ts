export interface FileRoot{id:string;name:string;kind:'cloud'|'local';path:string;revision:number}
export interface FileEntry{id:string;name:string;path:string;size:number;directory:boolean;modified?:number;unsupported?:boolean}
export interface FileStep{action:'mkdir'|'relocate';source:FileEntry;target:string}
export interface FilePlan{id:string;connectionId:string;revision:number;backendKind:string;root:string;operation:string;items:{source:FileEntry;target:string}[];steps:FileStep[];digest:string;expiresAt:string}
export interface FileResult{id:string;state:string;completedSteps:number;message:string;plan:FilePlan}
