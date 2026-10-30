import type {Subtitle} from '../api/subtitles';
import {subtitleLanguageLabel} from './subtitle-language';
export function subtitleRowTitle(row:Subtitle){return row.source==='embedded'?`内嵌 · 轨道 ${row.index} · ${subtitleLanguageLabel(row)} · ${row.extension.toUpperCase()}`:row.filename}
export function subtitleExtraTitle(row:Subtitle){const title=row.title?.trim();return row.source==='embedded'||!title||title===row.filename||title===subtitleLanguageLabel(row)||/^(english|英语|简体中文|繁体中文|中英双语|日语|韩语)$/i.test(title)?'':title}
