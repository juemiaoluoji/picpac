package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
	"pack_mate/internal/domain"
	"pack_mate/internal/repository"
	"testing"
	"time"
)

// TestPatchProfileMongoCommand checks atomic field updates and returned database state.
func TestPatchProfileMongoCommand(t *testing.T) {
	for _, name := range []string{"name", "clear birthday", "failure"} {
		t.Run(name, func(t *testing.T) {
			id := bson.NewObjectID()
			newName := "new"
			response := bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: bson.D{{Key: "_id", Value: id}, {Key: "st", Value: "created"}, {Key: "pf", Value: bson.D{{Key: "unm", Value: newName}, {Key: "gdr", Value: "female"}}}}}}
			if name == "failure" {
				response = bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 2}, {Key: "errmsg", Value: "failed"}}
			}
			deployment := drivertest.NewMockDeployment(response)
			opts := options.Client()
			opts.Deployment = deployment
			var command bson.Raw
			opts.SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { command = e.Command }})
			client, err := mongo.Connect(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Disconnect(context.Background()) }()
			patch := repository.UserProfilePatch{Username: &newName}
			if name == "clear birthday" {
				patch = repository.UserProfilePatch{BirthdaySet: true}
			}
			user, err := NewUserRepository(client.Database("test")).PatchProfile(context.Background(), id, patch, time.Now())
			if name == "failure" {
				if err == nil {
					t.Fatal("missing db error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			set := command.Lookup("update").Document().Lookup("$set").Document()
			elements, _ := set.Elements()
			if len(elements) != 2 {
				t.Fatalf("unexpected updated fields: %s", set)
			}
			if name == "name" && set.Lookup("pf.unm").StringValue() != newName {
				t.Fatal("name missing")
			}
			if name == "clear birthday" && set.Lookup("pf.bdy").Type != bson.TypeNull {
				t.Fatal("birthday not cleared")
			}
			if !command.Lookup("new").Boolean() {
				t.Fatal("did not request stored result")
			}
			if command.Lookup("query").Document().Lookup("st").StringValue() != string(domain.UserStatusCreated) {
				t.Fatal("inactive users not excluded")
			}
			if user.Profile.Gender != domain.UserGenderFemale {
				t.Fatal("did not return database state")
			}
		})
	}
}
