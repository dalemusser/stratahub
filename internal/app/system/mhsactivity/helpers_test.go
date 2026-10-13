package mhsactivity

import (
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"go.mongodb.org/mongo-driver/mongo"
)

func mhsdevicetestsStore(db *mongo.Database) *mhsdevicetests.Store { return mhsdevicetests.New(db) }
