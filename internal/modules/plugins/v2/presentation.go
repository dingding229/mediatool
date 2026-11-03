package v2

import (
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"strings"
)

// DescriptorName is a read projection; it never changes the signed snapshot.
// Manifest titles localize commands as well as services. Historical descriptors
// without a declaration still retain their original name or readable ID.
func DescriptorName(d contracts.Descriptor, titles ...map[string]string) string {
	if len(titles) > 0 {
		if title := strings.TrimSpace(titles[0][d.Kind+":"+d.ID]); title != "" {
			return title
		}
	}
	if name := strings.TrimSpace(d.Name); name != "" {
		return name
	}
	id := strings.TrimSpace(d.Cmd)
	if id == "" {
		id = strings.TrimSpace(d.ID)
	}
	return strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(id)), " ")
}

func runOperationName(r Run, metadata contracts.Snapshot, titles ...map[string]string) string {
	for _, d := range metadata.Descriptors {
		if d.Kind == string(r.Kind) && d.Handler == r.Handler {
			return strings.TrimPrefix(DescriptorName(d, titles...), "危险：")
		}
	}
	return map[RunKind]string{RunServiceKind: "后台检查", RunCommand: "状态查询", RunAction: "手动操作", RunEvent: "事件处理", RunAPI: "接口调用", RunChain: "扩展处理", RunModule: "模块处理", RunTool: "工具调用"}[r.Kind]
}
