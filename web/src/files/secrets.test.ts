import { describe,it,expect } from 'vitest'
import { changedSecrets } from './secrets'
describe('explicit credential echoes',()=>{
 it('does not replay an untouched refreshed token',()=>expect(changedSecrets({accessToken:'old',refreshToken:'before'},{accessToken:'old',refreshToken:'before'})).toEqual({}))
 it('sends only changed fields and keeps blank omission',()=>expect(changedSecrets({token:'new',password:'',apiKey:'same'},{apiKey:'same'})).toEqual({token:'new'}))
 it('preserves explicit header-blob clearing',()=>expect(changedSecrets({requestHeaders:'{}'},{requestHeaders:'{"Authorization":"owned"}'})).toEqual({requestHeaders:'{}'}))
})
