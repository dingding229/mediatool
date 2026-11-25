package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type RestoreOutputReceipt struct {
	State string              `json:"state"`
	Files []RestoreFileResult `json:"files"`
}

func (e Engine) recordOutputRestore(ctx context.Context, id string, offline bool, archive string, proofs []FileProof) (RestoreOutputReceipt, error) {
	receipt := RestoreOutputReceipt{State: "writing", Files: []RestoreFileResult{}}
	if err := writeOutputReceipt(ctx, e.DB, id, offline, receipt); err != nil {
		return receipt, err
	}
	files, err := e.restoreOutputs(ctx, archive, proofs)
	receipt = RestoreOutputReceipt{State: "recorded", Files: files}
	// Preserve the file outcome even if the original restore context timed out.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if recordErr := writeOutputReceipt(persist, e.DB, id, offline, receipt); recordErr != nil {
		return receipt, fmt.Errorf("输出回执未确认，禁止导入数据库：%w", recordErr)
	}
	return receipt, err
}

func writeOutputReceipt(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, id string, offline bool, receipt RestoreOutputReceipt) error {
	sql := `UPDATE ops_web_restores SET output_receipt=$2 WHERE id=$1 AND state='running'`
	if offline {
		sql = `UPDATE ops_requests SET output_receipt=$2 WHERE id=$1 AND state='executing'`
	}
	tag, err := q.Exec(ctx, sql, id, receipt)
	if err != nil {
		return err
	}
	return requireOneAffected(tag)
}

func withOutputOutcome(message, state string, receipt *RestoreOutputReceipt) string {
	if receipt == nil {
		return message
	}
	if receipt.State == "writing" {
		return message + "；逐文件结果未完整记录，可能已补文件；请核对磁盘，不自动删除"
	}
	var published, skipped int
	for _, f := range receipt.Files {
		if f.Published {
			published++
		}
		if f.Status == outputMismatch {
			skipped++
		}
	}
	if published > 0 {
		switch state {
		case "failed":
			message += fmt.Sprintf("；已补文件，DB 未提交（%d 个，保留，请人工核对）", published)
		case "rolled_back":
			message += fmt.Sprintf("；已补文件，DB 未提交目标状态（已补偿；%d 个文件保留，请人工核对）", published)
		case "unknown":
			message += fmt.Sprintf("；已补文件，DB 结果待核对（%d 个文件保留，不自动删除）", published)
		case "prepared":
			message += fmt.Sprintf("；已补文件，候选 DB 未切换（%d 个文件保留）", published)
		}
	}
	if skipped > 0 {
		message += fmt.Sprintf("；%d 个现存输出与备份不一致，已跳过并保留原字节", skipped)
	}
	return message
}
