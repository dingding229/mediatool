import { ref } from 'vue'
import { defineStore } from 'pinia'
import { request,post,APIError } from '../api/client'
export interface Administrator{id:string;username:string}
interface AuthResponse{authenticated:boolean;account:Administrator}
export const useAuthStore=defineStore('auth',()=>{
 const authenticated=ref(false),checked=ref(false),account=ref<Administrator|null>(null)
 async function ensure(){if(checked.value)return;try{const response=await request<AuthResponse>('/auth/session');authenticated.value=true;account.value=response.account}catch(error){if(!(error instanceof APIError)||error.status!==401)throw error;authenticated.value=false;account.value=null}finally{checked.value=true}}
 async function login(username:string,password:string){const response=await post<AuthResponse>('/auth/login',{username,password});authenticated.value=true;checked.value=true;account.value=response.account}
 async function register(username:string,password:string,confirmPassword:string){const response=await post<AuthResponse>('/auth/register',{username,password,confirmPassword});authenticated.value=true;checked.value=true;account.value=response.account}
 async function logout(){try{await post('/auth/logout',{})}finally{authenticated.value=false;checked.value=true;account.value=null}}
 async function changePassword(currentPassword:string,newPassword:string,confirmPassword:string){await post('/auth/password',{currentPassword,newPassword,confirmPassword});authenticated.value=false;checked.value=true;account.value=null}
 return{authenticated,checked,account,ensure,login,register,logout,changePassword}
})
