package session

import "time"

func (s *MemoryStorage) GetSession(key string) ([]byte, error) {
	deadline, err := RevocableSessionDeadline(key)
	if err != nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	full := s.buildKey(key)
	entry := s.data[full]
	if !time.Now().Before(deadline) || entry == nil {
		return nil, nil
	}
	if entry.isExpired() {
		delete(s.data, full)
		return nil, nil
	}
	return DecodeActiveSession(entry.data)
}

func (s *MemoryStorage) SetSessionIfActive(key string, value []byte, expiration time.Duration) error {
	deadline, err := RevocableSessionDeadline(key)
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return s.RevokeSession(key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !now.Before(deadline) {
		return ErrSessionExpired
	}
	full := s.buildKey(key)
	if existing := s.data[full]; existing != nil && !existing.isExpired() {
		if string(existing.data) == RevokedSessionValue {
			return ErrSessionRevoked
		}
		if _, err := DecodeActiveSession(existing.data); err != nil {
			return err
		}
	}
	expires := deadline
	if expiration > 0 && now.Add(expiration).Before(expires) {
		expires = now.Add(expiration)
	}
	encoded := append([]byte(ActiveSessionPrefix), value...)
	s.data[full] = &memoryEntry{data: encoded, expiresAt: expires}
	return nil
}

func (s *MemoryStorage) RevokeSession(key string) error {
	deadline, err := RevocableSessionDeadline(key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	full := s.buildKey(key)
	if !time.Now().Before(deadline) {
		delete(s.data, full)
		return nil
	}
	s.data[full] = &memoryEntry{data: []byte(RevokedSessionValue), expiresAt: deadline}
	return nil
}
