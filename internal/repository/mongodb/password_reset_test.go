package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
	"testing"
	"time"
)

// TestPasswordUpdateTransaction verifies transaction commands and conditional password writes with a simulated MongoDB server.
func TestPasswordUpdateTransaction(t *testing.T) {
	ok := bson.D{{Key: "ok", Value: 1}}
	updated := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}}
	for _, tc := range []struct {
		name      string
		responses []bson.D
		wantErr   bool
		last      string
		updates   int
	}{
		{"success", []bson.D{updated, updated, ok}, false, "commitTransaction", 2},
		{"revoke fails", []bson.D{updated, {{Key: "ok", Value: 0}, {Key: "code", Value: 2}, {Key: "errmsg", Value: "revoke failed"}}, ok}, true, "abortTransaction", 2},
		{"password changed", []bson.D{{{Key: "ok", Value: 1}, {Key: "n", Value: 0}}, ok}, true, "abortTransaction", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deployment := drivertest.NewMockDeployment(tc.responses...)
			opts := options.Client()
			opts.Deployment = deployment
			var commands []*event.CommandStartedEvent
			opts.SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { commands = append(commands, e) }})
			client, err := mongo.Connect(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Disconnect(context.Background()) }()
			repo := NewUserPasswordCredentialRepository(client.Database("password_reset_test"))
			err = repo.UpdatePasswordAndRevokeTokens(context.Background(), bson.NewObjectID(), "old-hash", "new-hash", "bcrypt", time.Now())
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected result: %v", err)
			}
			if len(commands) != tc.updates+1 || commands[len(commands)-1].CommandName != tc.last {
				t.Fatalf("unexpected transaction commands: %+v", commands)
			}
			first := commands[0].Command
			if !first.Lookup("startTransaction").Boolean() || first.Lookup("autocommit").Boolean() {
				t.Fatal("password write not transactional")
			}
			updates := first.Lookup("updates").Array()
			values, _ := updates.Values()
			if values[0].Document().Lookup("q").Document().Lookup("ph").StringValue() != "old-hash" {
				t.Fatal("missing concurrent password guard")
			}
			if tc.updates == 2 {
				second := commands[1].Command
				if second.Lookup("update").StringValue() != refreshTokenCollectionName || second.Lookup("txnNumber").Int64() != first.Lookup("txnNumber").Int64() {
					t.Fatal("revocation not in same transaction")
				}
			}
		})
	}
}
