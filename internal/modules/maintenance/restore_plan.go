package maintenance

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sort"
)

// These are executor/catalog records, not rollbackable business state. Policies,
// alerts/samples and all administrator/plugin/integration tables DO roll back.
var restoreMetadataTables = []string{"ops_mode", "ops_releases", "ops_backups", "ops_drills", "ops_requests", "ops_request_verifications", "ops_web_restores"}

func retainedRestoreTable(name string) bool {
	if name == "goose_db_version" || name == "ops_restore_commit" {
		return true
	}
	for _, t := range restoreMetadataTables {
		if t == name {
			return true
		}
	}
	return false
}

type RestoreTableChange struct {
	Name           string `json:"name"`
	CurrentRows    int64  `json:"currentRows"`
	BackupRows     int64  `json:"backupRows"`
	CurrentMissing bool   `json:"currentMissing"`
}

func (e Engine) restoreChanges(ctx context.Context, m Manifest) ([]RestoreTableChange, error) {
	rows, err := e.DB.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		return nil, err
	}
	current := map[string]bool{}
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		current[n] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for n := range current {
		names[n] = true
	}
	for n := range m.Tables {
		names[n] = true
	}
	sorted := []string{}
	for n := range names {
		if !retainedRestoreTable(n) {
			sorted = append(sorted, n)
		}
	}
	sort.Strings(sorted)
	out := []RestoreTableChange{}
	for _, n := range sorted {
		v := RestoreTableChange{Name: n, BackupRows: m.Tables[n].Rows, CurrentMissing: !current[n]}
		if current[n] {
			if err = e.DB.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{"public", n}.Sanitize()).Scan(&v.CurrentRows); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, nil
}
func (e Engine) restoreOrder(ctx context.Context) ([]string, error) {
	rows, err := e.DB.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		return nil, err
	}
	graph := map[string]map[string]bool{}
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		if !retainedRestoreTable(n) {
			graph[n] = map[string]bool{}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = e.DB.Query(ctx, `SELECT c.relname,p.relname FROM pg_constraint f JOIN pg_class c ON c.oid=f.conrelid JOIN pg_class p ON p.oid=f.confrelid WHERE f.contype='f' AND c.relnamespace='public'::regnamespace AND p.relnamespace='public'::regnamespace`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var child, parent string
		if err = rows.Scan(&child, &parent); err != nil {
			rows.Close()
			return nil, err
		}
		if child != parent && graph[parent] != nil {
			if graph[child] == nil {
				rows.Close()
				return nil, fmt.Errorf("保留账本依赖业务表%s，不能安全恢复", parent)
			}
			graph[child][parent] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	order := []string{}
	for len(graph) > 0 {
		ready := []string{}
		for n, deps := range graph {
			if len(deps) == 0 {
				ready = append(ready, n)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("业务表外键存在循环，拒绝禁用完整性校验")
		}
		sort.Strings(ready)
		for _, n := range ready {
			order = append(order, n)
			delete(graph, n)
			for _, deps := range graph {
				delete(deps, n)
			}
		}
	}
	return order, nil
}
