package onlineupdate

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// Semantic mount comparison is independent of Compose's wire representation.
// Binds implicitly create missing bind sources; typed mounts only do so with
// BindOptions.CreateMountpoint=true (Compose create_host_path).
type mountSpec struct {
	Type, Source, Target, Propagation                                                              string
	ReadOnly, CreateMountpoint, NonRecursive, ReadOnlyNonRecursive, ReadOnlyForceRecursive, NoCopy bool
}

func mountSpecs(h map[string]any) ([]mountSpec, error) {
	out := []mountSpec{}
	seen := map[string]bool{}
	add := func(m mountSpec) error {
		if m.Source == "" || !filepath.IsAbs(m.Target) || filepath.Clean(m.Target) != m.Target || seen[m.Target] {
			return ErrUnsafe
		}
		if m.Type == "bind" && (!filepath.IsAbs(m.Source) || filepath.Clean(m.Source) != m.Source) {
			return ErrUnsafe
		}
		seen[m.Target] = true
		out = append(out, m)
		return nil
	}
	binds, ok := strictStrings(h["Binds"])
	if !ok {
		return nil, ErrUnsafe
	}
	for _, b := range binds {
		a := strings.Split(b, ":")
		if len(a) < 2 || len(a) > 3 {
			return nil, ErrUnsafe
		}
		m := mountSpec{Type: "volume", Source: a[0], Target: a[1]}
		if filepath.IsAbs(m.Source) {
			m.Type = "bind"
			m.Propagation = "rprivate"
			m.CreateMountpoint = true
		}
		if len(a) == 3 {
			for _, v := range strings.Split(a[2], ",") {
				switch v {
				case "rw":
				case "ro":
					m.ReadOnly = true
				case "private", "rprivate", "shared", "rshared", "slave", "rslave":
					if m.Type != "bind" {
						return nil, ErrUnsafe
					}
					m.Propagation = v
				case "nocopy":
					if m.Type != "volume" {
						return nil, ErrUnsafe
					}
					m.NoCopy = true
				default:
					return nil, ErrUnsafe
				}
			}
		}
		if e := add(m); e != nil {
			return nil, e
		}
	}
	if h["Mounts"] != nil {
		list, ok := h["Mounts"].([]any)
		if !ok {
			return nil, ErrUnsafe
		}
		for _, raw := range list {
			v, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrUnsafe
			}
			for k := range v {
				if !has([]string{"Type", "Source", "Target", "ReadOnly", "BindOptions", "VolumeOptions", "Consistency"}, k) {
					return nil, ErrUnsafe
				}
			}
			if v["Consistency"] != nil && v["Consistency"] != "" {
				return nil, ErrUnsafe
			}
			m := mountSpec{Type: fmtString(v["Type"]), Source: fmtString(v["Source"]), Target: fmtString(v["Target"])}
			if x, exists := v["ReadOnly"]; exists {
				b, ok := x.(bool)
				if !ok {
					return nil, ErrUnsafe
				}
				m.ReadOnly = b
			}
			if m.Type != "bind" && m.Type != "volume" {
				return nil, ErrUnsafe
			}
			if m.Type == "bind" {
				m.Propagation = "rprivate"
				if v["VolumeOptions"] != nil {
					return nil, ErrUnsafe
				}
				if v["BindOptions"] != nil {
					o, ok := v["BindOptions"].(map[string]any)
					if !ok {
						return nil, ErrUnsafe
					}
					for k, x := range o {
						switch k {
						case "Propagation":
							p, ok := x.(string)
							if !ok {
								return nil, ErrUnsafe
							}
							if p != "" {
								if !has([]string{"private", "rprivate", "shared", "rshared", "slave", "rslave"}, p) {
									return nil, ErrUnsafe
								}
								m.Propagation = p
							}
						case "CreateMountpoint", "NonRecursive", "ReadOnlyNonRecursive", "ReadOnlyForceRecursive":
							b, ok := x.(bool)
							if !ok {
								return nil, ErrUnsafe
							}
							switch k {
							case "CreateMountpoint":
								m.CreateMountpoint = b
							case "NonRecursive":
								m.NonRecursive = b
							case "ReadOnlyNonRecursive":
								m.ReadOnlyNonRecursive = b
							case "ReadOnlyForceRecursive":
								m.ReadOnlyForceRecursive = b
							}
						default:
							return nil, ErrUnsafe
						}
					}
				}
				if m.ReadOnlyNonRecursive && m.ReadOnlyForceRecursive {
					return nil, ErrUnsafe
				}
			} else {
				if v["BindOptions"] != nil {
					return nil, ErrUnsafe
				}
				if v["VolumeOptions"] != nil {
					o, ok := v["VolumeOptions"].(map[string]any)
					if !ok {
						return nil, ErrUnsafe
					}
					for k, x := range o {
						if k != "NoCopy" {
							return nil, ErrUnsafe
						}
						b, ok := x.(bool)
						if !ok {
							return nil, ErrUnsafe
						}
						m.NoCopy = b
					}
				}
			}
			if e := add(m); e != nil {
				return nil, e
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out, nil
}

// The request is derived from the verified live inspect, never from the redacted
// plan or a second Compose version. It is held only in helper memory.
func (d *docker) appCreate(ctx context.Context) ([]byte, error) {
	if e := d.imageStartupPreflight(ctx); e != nil {
		return nil, e
	}
	b, e := json.Marshal(d.self.Config)
	if e != nil {
		return nil, e
	}
	var c map[string]any
	if json.Unmarshal(b, &c) != nil {
		return nil, ErrUnsafe
	}
	c["Image"] = d.target
	if d.self.Config["Hostname"] == d.self.ID[:12] {
		c["Hostname"] = ""
	}
	l, _ := c["Labels"].(map[string]any)
	if l == nil {
		return nil, ErrUnsafe
	}
	d.mu.Lock()
	imageLabels, _ := d.imageConfig["Labels"].(map[string]any)
	d.mu.Unlock()
	for k, v := range imageLabels {
		if strings.HasPrefix(k, "org.opencontainers.image.") {
			l[k] = v
		}
	}
	// Preserve all HostConfig values byte-for-byte, while also proving that their
	// mounts are supported and have exactly the same semantics.
	c["HostConfig"] = d.self.HostConfig
	ends := map[string]any{}
	for name, raw := range d.self.NetworkSettings.Networks {
		var ep map[string]any
		if json.Unmarshal(raw, &ep) != nil {
			return nil, ErrUnsafe
		}
		if ep["IPAMConfig"] != nil || ep["Links"] != nil || ep["DriverOpts"] != nil {
			return nil, ErrUnsafe
		}
		aliases := []string{}
		for _, a := range stringsValue(ep["Aliases"]) {
			if a == d.self.ID[:12] || a == d.self.ID {
				continue
			}
			if a != "app" && a != d.appName() {
				return nil, ErrUnsafe
			}
			aliases = append(aliases, a)
		}
		ends[name] = map[string]any{"Aliases": aliases}
	}
	c["NetworkingConfig"] = map[string]any{"EndpointsConfig": ends}
	return json.Marshal(c)
}

// Dry-run: all guards run against the exact request, with no HTTP write. The
// captured body never leaves process memory or enters the shared task journal.
func (d *docker) preflightCreate(body []byte) error {
	if bindings, ok := d.self.HostConfig["PortBindings"].(map[string]any); ok {
		for _, raw := range bindings {
			entries, ok := raw.([]any)
			if !ok {
				return ErrUnsafe
			}
			for _, entry := range entries {
				v, ok := entry.(map[string]any)
				if !ok || fmtString(v["HostPort"]) == "" || v["HostPort"] == "0" {
					return ErrUnsafe
				}
			}
		}
	}
	if _, e := mountSpecs(d.self.HostConfig); e != nil {
		return e
	}
	if !d.allowed("POST", "/containers/create?name="+d.appName(), body) {
		return ErrUnsafe
	}
	return nil
}
