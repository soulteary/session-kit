package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	session "github.com/soulteary/session-kit/v3"
)

type revocationEvaluator interface {
	Eval(context.Context, string, []string, ...any) *redis.Cmd
}

const saveActiveSessionScript = `
local clock=redis.call('TIME')
local now=tonumber(clock[1])*1000+math.floor(tonumber(clock[2])/1000)
local remaining=tonumber(ARGV[3])-now
if remaining<=0 then return -1 end
local current=redis.call('GET',KEYS[1])
if current==ARGV[2] then return 0 end
if current and string.sub(current,1,string.len(ARGV[4]))~=ARGV[4] then return -2 end
local idle=tonumber(ARGV[5])
if idle>0 and idle<remaining then remaining=idle end
redis.call('SET',KEYS[1],ARGV[1],'PX',remaining)
return 1
`
const revokeSessionScript = `
local clock=redis.call('TIME')
local now=tonumber(clock[1])*1000+math.floor(tonumber(clock[2])/1000)
local remaining=tonumber(ARGV[2])-now
if remaining<=0 then redis.call('DEL',KEYS[1]); return 1 end
redis.call('SET',KEYS[1],ARGV[1],'PX',remaining)
return 1
`

func (s *Storage) GetSession(key string) ([]byte, error) {
	deadline, err := session.RevocableSessionDeadline(key)
	if err != nil || !time.Now().Before(deadline) {
		return nil, nil
	}
	value, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	return session.DecodeActiveSession(value)
}

func (s *Storage) SetSessionIfActive(key string, value []byte, expiration time.Duration) error {
	deadline, err := session.RevocableSessionDeadline(key)
	if err != nil {
		return err
	}
	if !time.Now().Before(deadline) {
		return session.ErrSessionExpired
	}
	if len(value) == 0 {
		return s.RevokeSession(key)
	}
	evaluator, ok := s.client.(revocationEvaluator)
	if !ok || isNil(s.client) {
		return fmt.Errorf("redis client does not support atomic session revocation")
	}
	idle := expiration.Milliseconds()
	if expiration > 0 && idle == 0 {
		idle = 1
	}
	result, err := evaluator.Eval(context.Background(), saveActiveSessionScript, []string{s.buildKey(key)}, session.ActiveSessionPrefix+string(value), session.RevokedSessionValue, deadline.UnixMilli(), session.ActiveSessionPrefix, idle).Int()
	if err != nil {
		return fmt.Errorf("save active session: %w", err)
	}
	switch result {
	case 1:
		return nil
	case 0:
		return session.ErrSessionRevoked
	case -1:
		return session.ErrSessionExpired
	default:
		return session.ErrSessionEncoding
	}
}

func (s *Storage) RevokeSession(key string) error {
	deadline, err := session.RevocableSessionDeadline(key)
	if err != nil {
		return err
	}
	evaluator, ok := s.client.(revocationEvaluator)
	if !ok || isNil(s.client) {
		return fmt.Errorf("redis client does not support atomic session revocation")
	}
	if err := evaluator.Eval(context.Background(), revokeSessionScript, []string{s.buildKey(key)}, session.RevokedSessionValue, deadline.UnixMilli()).Err(); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
