import {countryChoices,languageLabel,genres} from '../classification/options';
export const genreSummary=(ids?:number[]|null)=>(ids||[]).map(id=>genres.find(g=>g.value===id)?.label||String(id)).join('、');
export const fullCountries=(codes?:string[]|null)=>(codes||[]).map(id=>countryChoices(codes||[]).find(c=>c.value===id)?.label||id).join('、');
export function countrySummary(codes?:string[]|null){const list=codes||[];return fullCountries(list.slice(0,3))+(list.length>3?`等 ${list.length} 个国家`:'')}
export const languagesSummary=(codes?:string[]|null)=>[...new Set((codes||[]).map(languageLabel))].join('、');
