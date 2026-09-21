package redisstore

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	session "github.com/soulteary/session-kit/v3"
)

// New already accepted a Cluster or Sentinel client, but only from a caller
// holding one it built itself. A caller that configures its backend instead --
// which is what session.NewStorage does -- could describe a standalone server
// and nothing else. These tests pin the configured path's reach.

// roundTrip exercises a storage end to end, so a constructor is judged by
// whether what it hands back actually talks to Redis.
func roundTrip(t *testing.T, storage *Storage) {
	t.Helper()

	if err := storage.Set("s1", []byte("payload"), time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := storage.Get("s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("Get = %q, want %q", got, "payload")
	}
	if err := storage.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestNewFromOptionsWithASingleAddr(t *testing.T) {
	mr := miniredis.RunT(t)

	storage, err := NewFromOptions(Options{Addr: mr.Addr(), KeyPrefix: "opts:"})
	if err != nil {
		t.Fatalf("NewFromOptions: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	roundTrip(t, storage)
	if !mr.Exists("opts:s1") {
		// Set then Delete above, so assert the prefix on a key that survives.
		if err := storage.Set("kept", []byte("x"), time.Hour); err != nil {
			t.Fatal(err)
		}
		if !mr.Exists("opts:kept") {
			t.Errorf("keys are not under the configured prefix; keys = %v", mr.Keys())
		}
	}
}

// Addrs is the field that makes a non-standalone deployment describable. One
// entry is still a single-node client, which is as far as miniredis can go --
// the Cluster and Sentinel shapes are covered by the compiled examples.
func TestNewFromOptionsWithAnAddrsList(t *testing.T) {
	mr := miniredis.RunT(t)

	storage, err := NewFromOptions(Options{Addrs: []string{mr.Addr()}})
	if err != nil {
		t.Fatalf("NewFromOptions: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	roundTrip(t, storage)
}

// Addrs wins over Addr, so a config carrying both (DefaultStorageConfig fills
// RedisAddr in) reaches the list the caller actually set.
func TestNewFromOptionsPrefersAddrsOverAddr(t *testing.T) {
	mr := miniredis.RunT(t)

	storage, err := NewFromOptions(Options{
		Addr:  "127.0.0.1:1", // nothing listens here
		Addrs: []string{mr.Addr()},
	})
	if err != nil {
		t.Fatalf("NewFromOptions: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	roundTrip(t, storage)
}

func TestNewFromOptionsDefaultsThePrefix(t *testing.T) {
	mr := miniredis.RunT(t)

	storage, err := NewFromOptions(Options{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("NewFromOptions: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	if got := storage.KeyPrefix(); got != DefaultKeyPrefix {
		t.Errorf("KeyPrefix = %q, want %q", got, DefaultKeyPrefix)
	}
}

func TestNewFromOptionsRejectsNoAddress(t *testing.T) {
	if _, err := NewFromOptions(Options{}); err == nil {
		t.Fatal("an Options with neither Addr nor Addrs returned no error")
	}
}

// Connectivity is verified before a storage is handed back, so a caller never
// receives one that was already known to be unreachable.
func TestNewFromOptionsVerifiesConnectivity(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()

	storage, err := NewFromOptions(Options{Addr: addr})
	if err == nil {
		_ = storage.Close()
		t.Fatal("connecting to a dead server returned no error")
	}
	if !strings.Contains(err.Error(), "Redis") {
		t.Errorf("err = %v, want it to name Redis", err)
	}
}

// NewFromConfig now delegates to NewFromOptions. Its four positional arguments
// must still land where they did.
func TestNewFromConfigStillWorks(t *testing.T) {
	mr := miniredis.RunT(t)

	storage, err := NewFromConfig(mr.Addr(), "", 0, "legacy:")
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	if got := storage.KeyPrefix(); got != "legacy:" {
		t.Errorf("KeyPrefix = %q, want %q", got, "legacy:")
	}
	roundTrip(t, storage)

	if _, err := NewFromConfig("127.0.0.1:1", "", 0, ""); err == nil {
		t.Error("NewFromConfig against a dead address returned no error")
	}
}

// The gap this change closes: session.NewStorage could only ever build a
// standalone client, because StorageConfig had nowhere to put anything else.
func TestNewStorageReachesRedisThroughAddrs(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithRedisAddrs(mr.Addr()).
		WithKeyPrefix("factory:")

	storage, err := session.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	if err := storage.Set("s1", []byte("payload"), time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !mr.Exists("factory:s1") {
		t.Errorf("the factory did not reach the configured server; keys = %v", mr.Keys())
	}
}

// DefaultStorageConfig fills RedisAddr in, so the registered builder must not
// let that default shadow an Addrs list the caller set.
func TestNewStorageAddrsBeatTheDefaultAddr(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithRedisAddrs(mr.Addr())
	if cfg.RedisAddr == "" {
		t.Fatal("this test is pointless unless DefaultStorageConfig sets RedisAddr")
	}

	storage, err := session.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	if err := storage.Set("s1", []byte("payload"), time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

// The Sentinel credentials are carried separately from the Redis password
// because they authenticate to different things; losing either in the handover
// would make an ACL-protected Sentinel unreachable with no hint why.
//
// Asserted against the mapping rather than through a failed dial: "some error
// came back" would pass just as happily with every field dropped, and costs
// seconds of Sentinel retries to learn nothing.
func TestStorageConfigReachesOptionsIntact(t *testing.T) {
	cfg := session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithKeyPrefix("sess:").
		WithRedisAddr("10.0.0.9:6379").
		WithRedisAddrs("10.0.0.1:26379", "10.0.0.2:26379").
		WithRedisMasterName("mymaster").
		WithRedisPassword("redis-secret").
		WithRedisSentinelAuth("sentinel-user", "sentinel-secret").
		WithRedisDB(3)

	want := Options{
		Addr:             "10.0.0.9:6379",
		Addrs:            []string{"10.0.0.1:26379", "10.0.0.2:26379"},
		MasterName:       "mymaster",
		Password:         "redis-secret",
		SentinelUsername: "sentinel-user",
		SentinelPassword: "sentinel-secret",
		DB:               3,
		KeyPrefix:        "sess:",
	}

	got := optionsFor(cfg)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("optionsFor(cfg) = %+v\nwant %+v", got, want)
	}
}

// WithRedisAddrs with no arguments clears the list, putting a configuration
// back on RedisAddr rather than leaving it with an empty non-nil slice.
func TestWithRedisAddrsClears(t *testing.T) {
	cfg := session.DefaultStorageConfig().WithRedisAddrs("a:1", "b:2").WithRedisAddrs()
	if len(cfg.RedisAddrs) != 0 {
		t.Errorf("RedisAddrs = %v, want empty", cfg.RedisAddrs)
	}
}
