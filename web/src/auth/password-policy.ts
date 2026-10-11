export const PASSWORD_MIN_LENGTH = 6
export const PASSWORD_MAX_LENGTH = 20
// Existing passwords may exceed the new creation limit. Login/current-password
// fields retain the previous limit so a policy change does not lock users out.
export const EXISTING_PASSWORD_MAX_LENGTH = 128
export function passwordPolicyError(password: string): string | null {
 const length = Array.from(password).length
 if (length < PASSWORD_MIN_LENGTH || length > PASSWORD_MAX_LENGTH || password.includes('\0')) return '密码需6–20个字符，可使用空格和中文'
 return null
}
