package printers

import (
	"sort"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	byID     map[string]*Printer
	defaults map[Channel]string
}

func NewRegistry() *Registry {
	return &Registry{
		byID:     make(map[string]*Printer),
		defaults: make(map[Channel]string),
	}
}

func (r *Registry) Upsert(p Printer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := p
	r.byID[p.ID] = &cp
}

func (r *Registry) Replace(printers []Printer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[string]*Printer, len(printers))
	for i := range printers {
		p := printers[i]
		next[p.ID] = &p
	}
	r.byID = next
}

func (r *Registry) Get(id string) (Printer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byID[id]
	if !ok {
		return Printer{}, false
	}
	return *p, true
}

func (r *Registry) List() []Printer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Printer, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		if out[i].IsThermal != out[j].IsThermal {
			return out[i].IsThermal
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// PickDefault returns the best printer to use when no explicit ID is given:
// 1) one explicitly marked IsDefault and thermal, 2) any thermal, 3) any default, 4) any.
func (r *Registry) PickDefault() (Printer, bool) {
	list := r.List()
	if len(list) == 0 {
		return Printer{}, false
	}
	for _, p := range list {
		if p.IsDefault && p.IsThermal {
			return p, true
		}
	}
	for _, p := range list {
		if p.IsThermal {
			return p, true
		}
	}
	for _, p := range list {
		if p.IsDefault {
			return p, true
		}
	}
	return list[0], true
}
