const numeric=(value:unknown):value is number=>typeof value==='number'&&Number.isFinite(value)
/** One compact-count convention across the UI: 万 / 千万, never mixed w/kw. */
export function formatCompactNumber(value:unknown):string {
 if(!numeric(value))return '—'
 const magnitude=Math.abs(value)
 if(magnitude>=1e7)return (value/1e7).toFixed(2)+'千万'
 if(magnitude>=1e4)return (value/1e4).toFixed(2)+'万'
 return value.toLocaleString('zh-CN',{maximumFractionDigits:2})
}
export const formatRatio=(value:unknown):string=>numeric(value)?value.toFixed(2):'—'
