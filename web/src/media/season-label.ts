import type {MediaItem} from './model';
export function seasonLabel(season:MediaItem,workTitle=''){
 const label=`第 ${season.season??0} 季`,title=season.title.trim();
 if(!title||title===workTitle||/^第\s*\d+\s*季$|^Season\s*\d+$/i.test(title))return label;
 return title.startsWith(label)?title:`${label} · ${title}`;
}
