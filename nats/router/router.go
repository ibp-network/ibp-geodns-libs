package router

import (
	"sync"

	"github.com/nats-io/nats.go"
)

// Module represents a pluggable message handler bound to one or more roles.
type Module interface {
	Name() string
	Handle(msg *nats.Msg) bool
}

// Registry stores the mapping between roles and their module stacks.
type Registry struct {
	mu          sync.RWMutex
	roleModules map[string][]Module
	global      []Module
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{
		roleModules: make(map[string][]Module),
	}
}

// Register attaches a module to a role. An empty role value registers the
// module globally (receives all messages regardless of role).
func (r *Registry) Register(role string, mod Module) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if role == "" {
		r.global = append(r.global, mod)
		return
	}

	r.roleModules[role] = append(r.roleModules[role], mod)
}

// Dispatch emits the message to the registered role modules.
// It returns true when a module reports handling the message.
func (r *Registry) Dispatch(role string, msg *nats.Msg) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, mod := range r.global {
		if mod.Handle(msg) {
			return true
		}
	}

	if mods, ok := r.roleModules[role]; ok {
		for _, mod := range mods {
			if mod.Handle(msg) {
				return true
			}
		}
	}
	return false
}
