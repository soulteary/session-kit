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
