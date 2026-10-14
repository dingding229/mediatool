package acquisition

import "strings"

// Overall progress is stage completion, not a fabricated network byte count.
func TransferProgress(state, phase string, bytes, hashed, total int64) (int, string) {
	if state == "imported" {
		return 100, "入库完成"
	}
	if state == "artifacts_pending" || strings.HasPrefix(phase, "output_resume") {
		return 95, "生成媒体库输出"
	}
	if state == "verified" || phase == "verified" {
		return 90, "云端大小及哈希已校验"
	}
	if phase == "verifying" {
		return 85, "校验云端文件"
	}
	if phase == "hashing" {
		p := 0
		if total > 0 {
			p = int(min(hashed, total) * 25 / total)
		}
		return 5 + p, "计算源文件哈希"
	}
	if state == "uploading" || state == "starting" {
		p := 0
		if total > 0 {
			p = int(min(bytes, total) * 50 / total)
		}
		return 30 + p, "传输文件（含秒传/复用）"
	}
	return 0, "等待处理"
}
