import {it,expect} from 'vitest';
import {failedJobTarget} from './model';
it('failure inbox links sites to their account and other jobs to the exact queue record',()=>{expect(failedJobTarget({id:'job',kind:'native.probe'})).toEqual({path:'/ops',query:{tab:'jobs',q:'job'}});expect(failedJobTarget({id:'site-job',kind:'site.metrics.refresh',payload:{accountId:'owned'}})).toEqual({path:'/sites',query:{tab:'accounts',accountId:'owned'}});expect(failedJobTarget({id:'missing',kind:'site.metrics.refresh'}).query.q).toBe('missing')});
