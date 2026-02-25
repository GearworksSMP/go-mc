package registry

import "slices"

type Registry[E any] struct {
	keys    map[string]int32
	values  []E
	indices map[*E]int32
	tags    map[string][]*E
}

func NewRegistry[E any]() Registry[E] {
	return Registry[E]{
		keys:    make(map[string]int32),
		values:  make([]E, 0, 256),
		indices: make(map[*E]int32),
		tags:    make(map[string][]*E),
	}
}

func (r *Registry[E]) Clear() {
	r.keys = make(map[string]int32)
	r.values = r.values[:0]
	r.indices = make(map[*E]int32)
	r.tags = make(map[string][]*E)
}

func (r *Registry[E]) Get(key string) (int32, *E) {
	id, ok := r.keys[key]
	if !ok {
		return -1, nil
	}
	return id, &r.values[id]
}

func (r *Registry[E]) GetByID(id int32) *E {
	if id >= 0 && id < int32(len(r.values)) {
		return &r.values[id]
	}
	return nil
}

func (r *Registry[E]) Put(key string, data E) (id int32, val *E) {
	id = int32(len(r.values))
	r.keys[key] = id
	r.values = append(r.values, data)
	val = &r.values[id]
	r.indices[val] = id
	return
}

// Tags

func (r *Registry[E]) Tag(tag string) []*E {
	return slices.Clone(r.tags[tag])
}

func (r *Registry[E]) ClearTags() {
	r.tags = make(map[string][]*E)
}

// Len returns the number of entries in the registry.
func (r *Registry[E]) Len() int {
	return len(r.values)
}

// Range calls fn for each entry in the registry, ordered by ID.
func (r *Registry[E]) Range(fn func(id int32, key string, value *E)) {
	// Build reverse map of id -> key for ordered iteration
	idToKey := make([]string, len(r.values))
	for key, id := range r.keys {
		idToKey[id] = key
	}
	for i := range r.values {
		fn(int32(i), idToKey[i], &r.values[i])
	}
}

// EncodableEntries returns all entries as RegistryEntry slice ordered by ID,
// suitable for encoding into registry data packets.
func (r *Registry[E]) EncodableEntries() []RegistryEntry {
	entries := make([]RegistryEntry, len(r.values))
	idToKey := make([]string, len(r.values))
	for key, id := range r.keys {
		idToKey[id] = key
	}
	for i := range r.values {
		entries[i] = RegistryEntry{Key: idToKey[i], Data: r.values[i]}
	}
	return entries
}
