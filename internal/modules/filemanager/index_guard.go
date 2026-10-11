package filemanager

import (
	"context"
	"path"
	"strings"
)

// A current path is not proof of the indexed media's identity. Never rebind an
// old asset to a replacement file that happens to occupy the same pathname.
func (e Engine) validateIndexed(ctx context.Context, root Root, b Backend, p Plan) error {
	if root.Kind != "cloud" {
		return nil
	}
	type record struct {
		id, full string
		size     int64
	}
	for _, item := range p.Items {
		if item.Source.ID == "" {
			continue
		}
		source := path.Join(root.Path, item.Source.Path)
		rows, err := e.DB.Query(ctx, `SELECT coalesce(provider_file_id,''),full_path,size_bytes FROM cloud_files WHERE storage_id=$1 AND (full_path=$2 OR left(full_path,length($2)+1)=$2||'/') LIMIT 501`, root.ID, source)
		if err != nil {
			return ErrUnavailable
		}
		records := []record{}
		for rows.Next() {
			var value record
			if rows.Scan(&value.id, &value.full, &value.size) != nil {
				rows.Close()
				return ErrUnavailable
			}
			records = append(records, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return ErrUnavailable
		}
		if len(records) > 500 {
			return ErrLimit
		}
		for _, value := range records {
			relative := strings.TrimPrefix(value.full, strings.TrimRight(root.Path, "/")+"/")
			actual, err := b.Find(ctx, relative)
			if err != nil || value.id == "" || actual.ID != value.id || actual.Size != value.size || actual.Directory {
				return ErrChanged
			}
		}
	}
	return nil
}
