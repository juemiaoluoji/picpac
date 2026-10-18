package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"pack_mate/internal/domain"
	"pack_mate/internal/repository"
	"time"
)

// PatchProfile updates only supplied profile fields and returns the stored user.
func (r *UserRepository) PatchProfile(ctx context.Context, id bson.ObjectID, patch repository.UserProfilePatch, at time.Time) (*domain.User, error) {
	set := bson.M{"uat": at}
	if patch.Username != nil {
		set["pf.unm"] = *patch.Username
	}
	if patch.Gender != nil {
		set["pf.gdr"] = *patch.Gender
	}
	if patch.BirthdaySet {
		set["pf.bdy"] = patch.Birthday
	}
	if patch.AvatarObjectKey != nil {
		set["pf.aok"] = *patch.AvatarObjectKey
	}
	if patch.AvatarDisplayObjectKey != nil {
		set["pf.adok"] = *patch.AvatarDisplayObjectKey
	}
	var doc userDocument
	err := r.collection.FindOneAndUpdate(ctx, bson.M{"_id": id, "st": domain.UserStatusCreated}, bson.M{"$set": set}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if err != nil {
		return nil, err
	}
	user := newDomainUser(doc)
	return &user, nil
}
