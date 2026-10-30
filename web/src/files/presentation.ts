import type {FileEntry} from './types';import {formatBytes} from '../utils/format-bytes';import {beijingTime} from '../presentation/time';
/** API modified is Unix nanoseconds (also used unchanged for mutation evidence). */
export function fileDescription(file:FileEntry){if(file.unsupported)return '不支持的文件类型';if(!file.directory)return '文件 · '+formatBytes(file.size);if(file.modified&&Number.isFinite(file.modified)){const time=new Date(file.modified/1e6);if(Number.isFinite(time.getTime()))return '文件夹 · 修改于 '+beijingTime(time.toISOString())}return '文件夹'}
