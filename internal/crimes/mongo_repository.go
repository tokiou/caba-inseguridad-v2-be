package crimes

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoRepository struct {
	collection *mongo.Collection
}

func NewMongoRepository(collection *mongo.Collection) *MongoRepository {
	return &MongoRepository{
		collection: collection,
	}
}

func (r *MongoRepository) FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error) {
	filter := bson.M{
		"location": bson.M{
			"$nearSphere": bson.M{
				"$geometry": bson.M{
					"type":        "Point",
					"coordinates": []float64{query.Lng, query.Lat},
				},
				"$maxDistance": query.RadiusMeters,
			},
		},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var crimes []Crime
	if err := cursor.All(ctx, &crimes); err != nil {
		return nil, err
	}

	return crimes, nil
}
