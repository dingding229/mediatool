import { post } from './client'

export const registerAdministrator = <T>(username: string, password: string, confirmPassword: string, initializationToken?: string) =>
  post<T>('/auth/register', { username, password, confirmPassword, initializationToken })
