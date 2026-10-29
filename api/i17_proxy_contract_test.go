package api_test

import "testing"

func annotateProxyContract(want object) {
	path := want["paths"].(object)["/api/v1/settings/network-proxy"].(object)
	schema := path["put"].(object)["requestBody"].(object)["content"].(object)["application/json"].(object)["schema"].(object)
	props := schema["properties"].(object)
	props["protocol"].(object)["enum"] = []string{"http", "socks5"}
	props["port"].(object)["description"] = "已配置代理端口 1–65535；未配置时为 0。"
	props["password"].(object)["writeOnly"] = true
	props["password"].(object)["description"] = "留空保留密码；更换端点或用户名须重填或明确 clearPassword。"
	props["clearPassword"].(object)["description"] = "明确清除已保存密码；不得与非空 password 同传。"
	schema["description"] = "schema 93；仅支持结构化字段。旧 url 请求字段严格返回 400 invalid_json。旧格式密文保留并返回 needsInput，已选代理失败不回退直连。"
}
func TestI17ProxyContractHidesPasswordAndRejectsURLShape(t *testing.T) {
	doc := readDocument(t)
	path := doc["paths"].(object)["/api/v1/settings/network-proxy"].(object)
	schema := path["put"].(object)["requestBody"].(object)["content"].(object)["application/json"].(object)["schema"].(object)
	props := schema["properties"].(object)
	for _, k := range []string{"protocol", "host", "port", "username", "password", "clearPassword", "clear", "tmdb", "sites", "revision"} {
		if props[k] == nil {
			t.Fatal("missing field", k)
		}
	}
	if props["url"] != nil || schema["additionalProperties"] != false || props["password"].(object)["writeOnly"] != true {
		t.Fatal("legacy URL or credential contract")
	}
	enums := props["protocol"].(object)["enum"].([]any)
	if len(enums) != 2 || enums[0] != "http" || enums[1] != "socks5" {
		t.Fatal(enums)
	}
	public := doc["components"].(object)["schemas"].(object)["modules_networkproxy_Record"].(object)["properties"].(object)
	if public["password"] != nil || public["url"] != nil || public["needsInput"] == nil || public["hasPassword"] == nil {
		t.Fatal("response fields")
	}
}
