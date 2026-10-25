import {expect,it} from 'vitest';
import {remainingTime} from './download-presentation';
import type {DownloadTask} from './model';
const task:DownloadTask={id:'owned',resourceId:'resource',title:'影片',state:'downloading',progress:.5,resourceSize:1073741824,downloadSpeed:1048576,syncedAt:'2026-10-26T00:00:00Z',qbState:'downloading'};
it('estimates only from a known resource size and current positive download speed',()=>{expect(remainingTime(task)).toBe('约 9 分钟');expect(remainingTime({...task,progress:1,downloadSpeed:0})).toBe('0 秒');expect(remainingTime({...task,resourceSize:40})).toBe('约 1 秒')});
it.each([{progress:null},{resourceSize:0},{downloadSpeed:0},{downloadSpeed:Infinity},{qbState:'pausedDL'},{syncedAt:undefined},{syncError:'断线'},{progress:NaN}])('does not invent an ETA from missing, stale or paused data: %s',patch=>{expect(remainingTime({...task,...patch})).toBe('—')});
