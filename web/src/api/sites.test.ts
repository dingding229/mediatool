import {expect,it} from 'vitest';
import {siteFavicon} from './sites';
it('only uses the authenticated fixed-template favicon API',()=>{expect(siteFavicon('/api/v1/pt/templates/hdsky/favicon?v=1.0')).toContain('/favicon');for(const x of ['https://tracker.example/favicon.ico','/api/v1/pt/templates/hdsky/icon?v=1','/api/v1/pt/templates/hdsky/favicon?url=http://localhost','/api/v1/pt/templates/../favicon'])expect(siteFavicon(x)).toBe('')});
