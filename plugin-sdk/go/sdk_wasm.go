//go:build wasip1

// Package pluginapi is the versioned Go/WASI Media Tool plugin capability SDK.
package pluginapi
import("encoding/json";"errors";"unsafe")
//go:wasmimport mediatool http_request
func httpRequest(ptr,size,out,capacity uint32)uint32
//go:wasmimport mediatool_host request
func hostRequest(ptr,size,out,capacity uint32)uint32
func call(input any,host bool)(json.RawMessage,error){raw,e:=json.Marshal(input);if e!=nil{return nil,e};out:=make([]byte,128<<10);ptr:=uint32(uintptr(unsafe.Pointer(&raw[0])));target:=uint32(uintptr(unsafe.Pointer(&out[0])));var n uint32;if host{n=hostRequest(ptr,uint32(len(raw)),target,uint32(len(out)))}else{n=httpRequest(ptr,uint32(len(raw)),target,64<<10)};if n==0||n>uint32(len(out)){return nil,errors.New("host capability unavailable or denied")};return out[:n],nil}
func Host(operation string,data any)(json.RawMessage,error){return call(map[string]any{"operation":operation,"data":data},true)}
func HTTP(target,method string,headers map[string]string,body string)(json.RawMessage,error){return call(map[string]any{"url":target,"method":method,"headers":headers,"body":body},false)}
