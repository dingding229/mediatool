package filemanager

import (
	"context"
	"errors"
	"media-tool/internal/modules/metadata"
	"path"
	"strings"
	"time"
)

var sidecarTypes = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".idx": true, ".sub": true, ".sup": true, ".smi": true, ".ttml": true, ".nfo": true}

func BuildPlan(ctx context.Context, b Backend, root Root, in Input, id string) (Plan, error) {
	p := Plan{Media: metadata.Item{Title: in.Title, Year: in.Year, MediaType: in.MediaType}, Category: in.Category, ID: id, ConnectionID: root.ID, Revision: root.Revision, Root: root.Path, OriginFingerprint: root.Origin, BackendKind: root.Kind, Operation: in.Operation, Items: []Item{}, Steps: []Step{}, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	source, err := Relative(in.Source)
	if err != nil {
		return p, err
	}
	destination, err := Relative(in.Destination)
	if err != nil {
		return p, err
	}
	if in.Operation != "mkdir" && (source == "." || source == TrashDirectory) {
		return p, ErrProtected
	}
	if in.Operation != "trash" && (destination == TrashDirectory || strings.HasPrefix(destination, TrashDirectory+"/")) {
		return p, ErrProtected
	}
	switch in.Operation {
	case "rename", "move", "organize", "mkdir", "trash":
	default:
		return p, ErrInvalid
	}
	if in.Operation == "mkdir" {
		if Name(in.Name) != nil || in.Name == TrashDirectory {
			return p, ErrInvalid
		}
		parent, err := b.Find(ctx, destination)
		if err != nil || !parent.Directory {
			return p, ErrMissing
		}
		target := path.Join(destination, in.Name)
		if err = absent(ctx, b, target); err != nil {
			return p, err
		}
		p.Steps = append(p.Steps, Step{Action: "mkdir", Target: target})
		p.Items = append(p.Items, Item{Target: target})
	} else {
		file, err := b.Find(ctx, source)
		if err != nil {
			return p, err
		}
		if file.ID == "" || file.Unsupported {
			return p, ErrProtected
		}
		name, err := FileName(in, file)
		if err != nil {
			return p, err
		}
		switch in.Operation {
		case "rename":
			destination = path.Dir(source)
		case "trash":
			destination = path.Join(TrashDirectory, id)
			name = file.Name
		case "organize":
			if file.Directory {
				return p, ErrInvalid
			}
			if in.MediaType != "movie" && in.MediaType != "series" {
				return p, ErrInvalid
			}
			if destination == "." {
				if Name(in.Title) != nil {
					return p, ErrInvalid
				}
				title := in.Title
				if in.Year != "" {
					if len(in.Year) != 4 || strings.Trim(in.Year, "0123456789") != "" {
						return p, ErrInvalid
					}
					title += " (" + in.Year + ")"
				}
				group := "电影"
				if in.MediaType == "series" {
					group = "电视剧"
				}
				if in.Category != "" {
					if Name(in.Category) != nil {
						return p, ErrInvalid
					}
					group = path.Join(group, in.Category)
				}
				destination = path.Join(group, title)
				if in.MediaType == "series" {
					if in.Season < 0 || in.Season > 999 {
						return p, ErrInvalid
					}
					destination = path.Join(destination, "Season "+pad(in.Season))
				}
			}
		}
		target := path.Join(destination, name)
		if target == source || file.Directory && (target == source || strings.HasPrefix(target, source+"/")) {
			return p, ErrInvalid
		}
		if err = addParents(ctx, b, &p, destination, in.Operation == "organize" || in.Operation == "trash"); err != nil {
			return p, err
		}
		files := []File{file}
		targets := []string{target}
		if (in.Operation == "organize" || in.Operation == "rename") && in.IncludeSidecars && !file.Directory {
			siblings, err := b.List(ctx, path.Dir(source))
			if err != nil {
				return p, err
			}
			oldStem := strings.TrimSuffix(file.Name, path.Ext(file.Name))
			newStem := strings.TrimSuffix(name, path.Ext(name))
			for _, sibling := range siblings {
				if sibling.Directory || sibling.Unsupported || sibling.Path == file.Path || !sidecarTypes[strings.ToLower(path.Ext(sibling.Name))] {
					continue
				}
				stem := strings.TrimSuffix(sibling.Name, path.Ext(sibling.Name))
				if stem != oldStem && !strings.HasPrefix(stem, oldStem+".") {
					continue
				}
				if len(files) >= 65 {
					return p, ErrInvalid
				}
				suffix := strings.TrimPrefix(sibling.Name, oldStem)
				files = append(files, sibling)
				targets = append(targets, path.Join(destination, newStem+suffix))
			}
			// VobSub pairs must remain complete rather than moving an unusable idx.
			for _, f := range files {
				if strings.EqualFold(path.Ext(f.Name), ".idx") {
					partner := strings.TrimSuffix(f.Name, path.Ext(f.Name)) + ".sub"
					found := false
					for _, g := range files {
						if strings.EqualFold(g.Name, partner) {
							found = true
						}
					}
					if !found {
						return p, ErrInvalid
					}
				}
			}
		}
		seen := map[string]bool{}
		for i, f := range files {
			to := targets[i]
			if seen[to] || f.ID == "" {
				return p, ErrInvalid
			}
			seen[to] = true
			if err = absent(ctx, b, to); err != nil {
				return p, err
			}
			p.Items = append(p.Items, Item{Source: f, Target: to})
			if root.Kind == "cloud" && path.Base(f.Path) != path.Base(to) && path.Dir(f.Path) != path.Dir(to) {
				intermediate := path.Join(path.Dir(f.Path), path.Base(to))
				if err = absent(ctx, b, intermediate); err != nil {
					return p, err
				}
				p.Steps = append(p.Steps, Step{Action: "relocate", Source: f, Target: intermediate})
				renamed := f
				renamed.Name = path.Base(intermediate)
				renamed.Path = intermediate
				p.Steps = append(p.Steps, Step{Action: "relocate", Source: renamed, Target: to})
			} else {
				p.Steps = append(p.Steps, Step{Action: "relocate", Source: f, Target: to})
			}
		}
	}
	p.Digest = fingerprint(p)
	return p, nil
}
func absent(ctx context.Context, b Backend, p string) error {
	_, err := b.Find(ctx, p)
	if errors.Is(err, ErrMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrConflict
}
func addParents(ctx context.Context, b Backend, p *Plan, directory string, create bool) error {
	if directory == "." {
		return nil
	}
	f, err := b.Find(ctx, directory)
	if err == nil {
		if !f.Directory || f.Unsupported {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, ErrMissing) {
		return err
	}
	if !create {
		return ErrMissing
	}
	if err = addParents(ctx, b, p, path.Dir(directory), create); err != nil {
		return err
	}
	for _, step := range p.Steps {
		if step.Action == "mkdir" && step.Target == directory {
			return nil
		}
	}
	p.Steps = append(p.Steps, Step{Action: "mkdir", Target: directory})
	return nil
}
