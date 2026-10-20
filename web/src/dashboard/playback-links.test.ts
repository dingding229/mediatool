import {expect,it} from 'vitest'
import {failedJobTarget} from './model'
it('routes only actual native worker kinds to preprocessing, and keeps the exact queue record link',()=>{expect(failedJobTarget({id:'job',kind:'native.probe'})).toEqual({path:'/playback',query:{tab:'processing',kind:'probe',jobId:'job'}});expect(failedJobTarget({id:'still-job',kind:'native.still'}).query.kind).toBe('still');expect(failedJobTarget({id:'other',kind:'other'})).toEqual({path:'/ops',query:{tab:'jobs',q:'other'}})})
