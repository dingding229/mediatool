import {expect,it} from 'vitest'
import {APIError} from '../api/client'
import {operationError} from './operation-error'
it('uses fixed codes and a valid generated requestId, not provider/private strings',()=>{
 const v=operationError(new APIError(400,'secret-code-sentinel','secret-body-sentinel','secret-requestid-sentinel'))
 expect(v).toEqual({uncertain:false,label:'request_rejected · requestId=不可用'})
 expect(JSON.stringify(v)).not.toContain('sentinel')
})
it('does not mistake an unreadable successful reply or a server timeout for a rejected install',()=>{
 for(const e of [new APIError(200,'invalid_response','invalid'),new APIError(200,'plugin_conflict','malformed-success'),new APIError(408,'request_failed','timeout'),new APIError(503,'plugins_unavailable','unavailable'),new TypeError('private-network-message')])expect(operationError(e).uncertain).toBe(true)
})
it('keeps definitive host rejection separate from unknown commit outcomes',()=>{
 for(const status of [400,401,403,404,409,429])expect(operationError(new APIError(status,'plugin_conflict','private','abcdef0123456789abcdef01'))).toEqual({uncertain:false,label:'plugin_conflict · requestId=abcdef0123456789abcdef01'})
})
