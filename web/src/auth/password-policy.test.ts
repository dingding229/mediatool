import { describe,expect,it } from 'vitest'
import { PASSWORD_MIN_LENGTH,PASSWORD_MAX_LENGTH,EXISTING_PASSWORD_MAX_LENGTH,passwordPolicyError } from './password-policy'
describe('password policy',()=>{
 it('accepts inclusive 6 and 20 character boundaries',()=>{expect(PASSWORD_MIN_LENGTH).toBe(6);expect(PASSWORD_MAX_LENGTH).toBe(20);for(const password of ['abc123','x'.repeat(20),'中'.repeat(6),'中'.repeat(20),'🔒'.repeat(6)])expect(passwordPolicyError(password)).toBeNull()})
 it('rejects 5 and 21 characters without trimming passwords',()=>{for(const password of ['x'.repeat(5),'x'.repeat(21),'中'.repeat(5),'中'.repeat(21),'abc12\0'])expect(passwordPolicyError(password)).not.toBeNull();expect(passwordPolicyError(' abc12')).toBeNull();expect(EXISTING_PASSWORD_MAX_LENGTH).toBe(128)})
})
