export function connectionOwner(kind:string):string{return({ptsite:'/sites',emby:'/connections',clouddrive2:'/connections',open115:'/connections',qbittorrent:'/downloads',tmdb:'/connections',telegram:'/connections',local:'/files'}as Record<string,string>)[kind]??'/connections'}
export function connectionLabel(kind:string):string{return({ptsite:'PT 站点',emby:'媒体库',clouddrive2:'云盘',open115:'云盘 · 115 授权',qbittorrent:'下载与传输',tmdb:'连接 · TMDB',telegram:'通知',local:'本地文件'}as Record<string,string>)[kind]??kind}

export function connectionTab(kind:string){return ({emby:"emby",clouddrive2:"storage",open115:"storage",tmdb:"aux",telegram:"notifications"} as Record<string,string>)[kind]??"overview"}
