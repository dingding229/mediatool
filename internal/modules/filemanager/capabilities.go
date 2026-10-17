package filemanager

import (
	"context"
	deferutil "media-tool/internal/platform/cleanup"
	"path"
	"path/filepath"
	"strings"
)

// Capabilities describe the application's allowed actions, not a promise that a
// remote provider will accept a write. No write probes are made while browsing.
type Capabilities struct {
	Read     bool   `json:"read"`
	Write    bool   `json:"write"`
	Move     bool   `json:"move"`
	Recycle  bool   `json:"recycle"`
	ReadOnly bool   `json:"readOnly"`
	Role     string `json:"role"`
	Reason   string `json:"reason,omitempty"`
}

func safeLocalRoot(raw string) bool {
	p, err := filepath.Abs(raw)
	// macOS canonical /private/var is an OS alias, not the application private directory.
	if strings.HasPrefix(p, "/private/var/") {
		p = strings.TrimPrefix(p, "/private")
	}
	if err != nil || p == "/" || privateLocalPath(p) {
		return false
	}
	for _, deny := range []string{"/dev", "/proc", "/sys", "/run", "/var/run", "/var/lib/docker", "/var/lib/postgresql", "/var/lib/redis"} {
		if p == deny || strings.HasPrefix(p, deny+"/") {
			return false
		}
	}
	return true
}

func (e Engine) capability(ctx context.Context, root Root, relative string, b Backend, file *File) (Capabilities, error) {
	c := Capabilities{Read: true, Write: true, Move: true, Recycle: true, Role: "workspace"}
	if file != nil && file.Unsupported {
		return Capabilities{ReadOnly: true, Role: "workspace", Reason: "符号链接或非普通文件不能操作"}, nil
	}
	if root.Kind == "cloud" {
		c.Role = "cloud-target"
	}
	if root.Kind == "local" {
		full := filepath.Join(root.Path, filepath.FromSlash(relative))
		cfg := e.Connections.Config
		for _, private := range []string{cfg.LogDir, cfg.PasswordResetFile} {
			if private == "" {
				continue
			}
			absolute, err := filepath.Abs(private)
			if err == nil && (full == absolute || strings.HasPrefix(full, absolute+string(filepath.Separator))) {
				c.Read = false
				c.Reason = "私密系统路径"
				break
			}
		}
		if privateLocalPath(relative) {
			c.Read = false
			c.Reason = "私密系统路径"
		}
		if cfg.STRMOutputDir != "" {
			output, err := filepath.Abs(cfg.STRMOutputDir)
			if err == nil && (full == output || strings.HasPrefix(full, output+string(filepath.Separator))) {
				c.Role = "managed-output"
				c.Reason = "托管输出请使用输出修复与归档流程"
			}
		}
		rows, err := e.DB.Query(ctx, `SELECT settings->>'localPath' FROM integrations WHERE kind='qbittorrent' AND coalesce(settings->>'localPath','')<>''`)
		if err != nil {
			return c, ErrUnavailable
		}
		for rows.Next() {
			var source string
			if err = rows.Scan(&source); err != nil {
				rows.Close()
				return c, ErrUnavailable
			}
			source, err = filepath.Abs(source)
			if err == nil && (full == source || strings.HasPrefix(full, source+string(filepath.Separator))) {
				c.Role = "download-source"
				c.Reason = "下载及做种原文件只读保留"
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return c, ErrUnavailable
		}
		if local, ok := b.(*Local); ok {
			writable, err := local.writable(relative)
			if err != nil && c.Read {
				return c, err
			}
			if !writable && c.Reason == "" {
				c.Reason = "目录权限或挂载为只读"
			}
		}
	}
	if relative == TrashDirectory || strings.HasPrefix(relative, TrashDirectory+"/") {
		c.Role = "archive"
		c.Recycle = false
	}
	if file != nil && file.Unsupported {
		c.Reason = "符号链接或非普通文件不能操作"
		c.Read = false
	}
	if !c.Read || c.Reason != "" {
		c.Write = false
		c.Move = false
		c.Recycle = false
		c.ReadOnly = true
	}
	if file != nil && file.Directory {
		if err := e.protected(ctx, root, relative); err == ErrProtected {
			c.Move = false
			c.Recycle = false
			if c.Reason == "" {
				c.Reason = "目录包含受保护路径，不能整体移动或回收"
			}
		} else if err != nil {
			return c, err
		}
	}
	if relative == "." {
		c.Move = false
		c.Recycle = false
	}
	return c, nil
}

func (l *Local) writable(raw string) (bool, error) {
	p, err := Relative(raw)
	if err != nil {
		return false, err
	}
	// Plans may create a missing directory tree. Check the nearest existing parent
	// through os.Root, never follow a symlink or inspect an unrestricted host path.
	for {
		info, err := l.Find(context.Background(), p)
		if err == nil {
			if !info.Directory {
				p = path.Dir(p)
				continue
			}
			dir, err := l.Root.Open(p)
			if err != nil {
				return false, ErrUnavailable
			}
			defer deferutil.Close(dir, "Read/descriptor teardown only; primary operations handle their own I/O result")
			st, err := dir.Stat()
			if err != nil {
				return false, ErrUnavailable
			}
			if st.Mode().Perm()&0222 == 0 {
				return false, nil
			}
			return directoryWritable(dir)
		}
		if err != ErrMissing {
			return false, err
		}
		if p == "." {
			return false, ErrUnavailable
		}
		p = path.Dir(p)
	}
}

func (e Engine) requireWritable(ctx context.Context, root Root, b Backend, step Step) error {
	paths := []string{path.Dir(step.Target)}
	if step.Action != "mkdir" {
		paths = append(paths, path.Dir(step.Source.Path))
	}
	for _, p := range paths {
		cap, err := e.capability(ctx, root, p, b, nil)
		if err != nil {
			return err
		}
		if !cap.Read {
			return ErrProtected
		}
		if !cap.Write {
			if cap.Role == "download-source" || cap.Role == "managed-output" {
				return ErrProtected
			}
			return ErrReadOnly
		}
	}
	return nil
}

func (e Engine) privateBrowsePath(root Root, relative string) bool {
	if privateLocalPath(relative) {
		return true
	}
	full := filepath.Join(root.Path, filepath.FromSlash(relative))
	for _, private := range []string{e.Connections.Config.LogDir, e.Connections.Config.PasswordResetFile} {
		if private == "" {
			continue
		}
		absolute, err := filepath.Abs(private)
		if err == nil && (full == absolute || strings.HasPrefix(full, absolute+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
