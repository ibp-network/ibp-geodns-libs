package config

func GetMember(name string) (Member, bool) {
	if cfg == nil {
		return Member{}, false
	}

	cfg.mu.RLock()
	member, exists := cfg.data.Members[name]
	cfg.mu.RUnlock()
	return member, exists
}

func SetMember(name string, member Member) {
	if cfg == nil {
		return
	}

	cfg.mu.Lock()
	if cfg.data.Members == nil {
		cfg.data.Members = make(map[string]Member)
	}
	cfg.data.Members[name] = member
	cfg.mu.Unlock()
}

func DeleteMember(name string) {
	if cfg == nil {
		return
	}

	cfg.mu.Lock()
	delete(cfg.data.Members, name)
	cfg.mu.Unlock()
}

func ListMembers() map[string]Member {
	if cfg == nil {
		return map[string]Member{}
	}

	cfg.mu.RLock()
	membersCopy := make(map[string]Member, len(cfg.data.Members))
	for k, v := range cfg.data.Members {
		membersCopy[k] = v
	}
	cfg.mu.RUnlock()
	return membersCopy
}
