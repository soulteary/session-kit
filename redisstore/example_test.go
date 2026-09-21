package redisstore_test

import (
	"context"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	session "github.com/soulteary/session-kit/v3"
	"github.com/soulteary/session-kit/v3/redisstore"
)

// New takes an existing client, which is the usual path: the storage does not
// decide how the connection is configured. In a real service `addr` is the
// Redis server; here it is an in-process fake so the example runs.
func Example() {
	server, err := miniredis.Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()

	storage := redisstore.New(client, "session:")
	manager := session.NewManager(storage, session.DefaultConfig().WithExpiration(time.Hour))

	record := manager.CreateSession("sid-123")
	record.Authenticated = true
	record.UserID = "user-42"
	if err := manager.SaveSession(record); err != nil {
		fmt.Println(err)
		return
	}

	loaded, err := manager.LoadSession("sid-123")
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(loaded.UserID, loaded.IsAuthenticated())
	fmt.Println(server.Exists("session:sid-123"))
	// Output:
	// user-42 true
	// true
}

// NewStore is the generic key-value session store behind session.KVManager,
// for services that want Create/Get/Set/Delete with a TTL and an opaque map
// rather than a SessionData record.
func ExampleNewStore() {
	server, err := miniredis.Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()

	manager := session.NewKVManager(redisstore.NewStore(client, "otp:"), 15*time.Minute)
	ctx := context.Background()

	id, err := manager.Create(ctx, map[string]interface{}{"phone": "+100"}, 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	record, err := manager.Get(ctx, id)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(record.Data["phone"])

	if err := manager.Delete(ctx, id); err != nil {
		fmt.Println(err)
		return
	}

	exists, err := manager.Exists(ctx, id)
	fmt.Println(exists, err)
	// Output:
	// +100
	// false <nil>
}

// ExampleNewFromOptions builds a storage against a standalone server without
// the caller constructing a client. It is [NewFromConfig] with room for the
// deployments those four positional arguments cannot describe.
func ExampleNewFromOptions() {
	server, err := miniredis.Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer server.Close()

	storage, err := redisstore.NewFromOptions(redisstore.Options{
		Addr:      server.Addr(),
		KeyPrefix: "myapp:session:",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = storage.Close() }()

	if err := storage.Set("abc123", []byte("payload"), time.Hour); err != nil {
		fmt.Println(err)
		return
	}
	value, err := storage.Get("abc123")
	fmt.Printf("%s %v\n", value, err)

	// Output:
	// payload <nil>
}

// ExampleNewFromOptions_cluster points the storage at a Redis Cluster. Two or
// more addresses with no master name select a cluster client.
//
// It has no Output comment because it needs a real cluster; go test compiles
// it, which is what keeps it honest.
func ExampleNewFromOptions_cluster() {
	storage, err := redisstore.NewFromOptions(redisstore.Options{
		Addrs:     []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"},
		KeyPrefix: "myapp:session:",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = storage.Close() }()
}

// ExampleNewFromOptions_sentinel points the storage at a Sentinel-managed
// failover setup. The addresses are the Sentinel nodes, not the Redis servers,
// and the two sets of credentials authenticate to different things: Password
// to the master Sentinel points at, SentinelUsername/SentinelPassword to
// Sentinel itself.
//
// No Output comment, for the same reason as the cluster example.
func ExampleNewFromOptions_sentinel() {
	storage, err := redisstore.NewFromOptions(redisstore.Options{
		Addrs:            []string{"10.0.0.1:26379", "10.0.0.2:26379"},
		MasterName:       "mymaster",
		Password:         "the-redis-password",
		SentinelUsername: "sentinel-user",
		SentinelPassword: "the-sentinel-password",
		KeyPrefix:        "myapp:session:",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = storage.Close() }()
}

// ExampleNewFromOptions_fromConfiguration takes the path a service does when
// it picks its backend from configuration rather than constructing one. Before
// RedisAddrs and RedisMasterName existed, this path could only ever reach a
// standalone server.
func ExampleNewFromOptions_fromConfiguration() {
	server, err := miniredis.Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer server.Close()

	// In a real service these come from the environment or a config file; a
	// Cluster deployment sets several addresses, a Sentinel one also sets the
	// master name.
	cfg := session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithRedisAddrs(server.Addr()).
		WithKeyPrefix("myapp:session:")

	storage, err := session.NewStorage(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = storage.Close() }()

	if err := storage.Set("abc123", []byte("payload"), time.Hour); err != nil {
		fmt.Println(err)
		return
	}
	value, err := storage.Get("abc123")
	fmt.Printf("%s %v\n", value, err)

	// Output:
	// payload <nil>
}
