package v1

import "encoding/json"

func (r *Resource) ResourceKey() string {
	return r.ID
}

// DeepCopy return a copy of resource
func (r *Resource) DeepCopy() *Resource {
	var out Resource
	data, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(data, &out)
	return &out
}

func (rs Resources) Index() map[string]*Resource {
	m := make(map[string]*Resource)
	for i := range rs {
		m[rs[i].ResourceKey()] = &rs[i]
	}
	return m
}

// GVKIndex returns a map of GVK to resources, for now, only Kubernetes resources.
func (rs Resources) GVKIndex() map[string][]*Resource {
	m := make(map[string][]*Resource)
	for i := range rs {
		resource := &rs[i]
		if resource.Type != Kubernetes {
			continue
		}
		gvk := resource.Extensions[ResourceExtensionGVK].(string)
		m[gvk] = append(m[gvk], resource)
	}
	return m
}

func (rs Resources) Len() int      { return len(rs) }
func (rs Resources) Swap(i, j int) { rs[i], rs[j] = rs[j], rs[i] }
func (rs Resources) Less(i, j int) bool {
	switch {
	case rs[i].ID != rs[j].ID:
		return rs[i].ID < rs[j].ID
	default:
		return false
	}
}
