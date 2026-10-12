//go:build wasip1

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type input struct {
	Event struct {
		Kind     string            `json:"kind"`
		Settings map[string]string `json:"settings"`
		Data     map[string]any    `json:"data"`
	} `json:"event"`
	Data map[string]any `json:"data"`
}

func main() {
	var in input
	if json.NewDecoder(os.Stdin).Decode(&in) != nil {
		os.Exit(1)
	}
	data := in.Data
	if data == nil {
		data = map[string]any{}
	}
	count, _ := data["count"].(float64)
	kind := in.Event.Kind
	result := map[string]any{}
	switch kind {
	case "plugin.init":
		data["initialized"] = true
		result["message"] = "插件初始化完成"
	case "plugin.stop":
		data["stopped"] = true
		result["message"] = "插件服务已停用"
	case "plugin.service":
		count++
		data["count"] = count
		result["message"] = "定时任务执行完成"
	case "plugin.command":
		if in.Event.Data["command"] == "increment" {
			count++
			data["count"] = count
		}
		result["message"] = "命令已执行"
		result["count"] = count
	case "plugin.api":
		result["count"] = count
		result["name"] = in.Event.Settings["name"]
	case "plugin.test":
		_, fileError := os.ReadFile("/etc/passwd")
		result["filesystemDenied"] = fileError != nil
		result["environmentDenied"] = os.Getenv("APP_SECRET_KEY") == ""
		result["message"] = "WASM沙箱测试通过，不修改媒体"
		result["count"] = count
	default:
		result["title"] = in.Event.Settings["name"]
		result["description"] = "示例插件的独立页面，数据由Go/WASM逻辑返回。"
		result["metrics"] = []map[string]any{{"label": "执行计数", "value": count}, {"label": "已初始化", "value": data["initialized"]}}
		result["rows"] = []map[string]any{{"功能": "命令", "状态": "可用"}, {"功能": "定时服务", "状态": "启用后每60秒"}, {"功能": "管理员API", "状态": "仅已登录用户"}}
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"result": result, "data": data, "logs": []map[string]string{{"level": "info", "message": fmt.Sprint("示例插件处理事件：", kind)}}})
}
