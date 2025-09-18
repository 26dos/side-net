package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	mmongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ClaimsTaskResult is a minimal projection of claims_task_result.
type ClaimsTaskResult struct {
	ID        any        `bson:"_id"`
	Task      TaskDoc    `bson:"task"`
	Result    ResultDoc  `bson:"result"`
	CreatedAt time.Time  `bson:"created_at"`
}

type TaskDoc struct {
	Module   string        `bson:"module"`
	Provider ProviderDoc   `bson:"provider"`
	Content  ContentDoc    `bson:"content"`
}

type ProviderDoc struct {
	ID         string   `bson:"id"`
	PeerID     string   `bson:"peer_id"`
	Multiaddrs []string `bson:"multiaddrs"`
}

type ContentDoc struct {
	CID string `bson:"cid"`
}

type ResultDoc struct {
	Success bool   `bson:"success"`
	Code    string `bson:"error_code"`
	Msg     string `bson:"error_message"`
}

type Repo struct {
	DB        *mmongo.Database
	Results   *mmongo.Collection
	Paths     *mmongo.Collection // side_window_paths
}

// New connects to Mongo and prepares collections.
func New(ctx context.Context, uri, db string) (*Repo, error) {
	c, err := mmongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil { return nil, err }
	d := c.Database(db)
	r := &Repo{
		DB:        d,
		Results:   d.Collection("claims_task_result"),
		Paths:     d.Collection("side_window_paths"),
	}
	_, _ = r.Paths.Indexes().CreateOne(ctx, mmongo.IndexModel{
		Keys:    bson.D{{Key: "piece", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_piece"),
	})
	return r, nil
}

// FindOneSuccess returns the most recent success record for module http.
func (r *Repo) FindOneSuccess(ctx context.Context) (*ClaimsTaskResult, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	var out ClaimsTaskResult
	err := r.Results.FindOne(ctx, bson.M{"result.success": true, "task.module": "http"}, opts).Decode(&out)
	if err != nil { return nil, err }
	return &out, nil
}

// MarkFailed flips a record to failed with an error code/message.
func (r *Repo) MarkFailed(ctx context.Context, id any, code, msg string) error {
	_, err := r.Results.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{
			"result.success":       false,
			"result.error_code":    code,
			"result.error_message": msg,
		},
	})
	return err
}

// WindowDoc is stored in side_window_paths with exported window paths.
type WindowDoc struct {
	Piece     string        `bson:"piece"`
	BuiltAt   time.Time     `bson:"built_at"`
	HashAlgo  string        `bson:"hash_algo"`
	Arity     int           `bson:"arity"`
	LeafSize  int           `bson:"leaf_size"`
	Root      string        `bson:"root"`
	WindowSzB int           `bson:"window_size_bytes"`
	Paths     []any         `bson:"paths"`
	Meta      bson.M        `bson:"meta"`
}

// SaveWindowPaths upserts a window-paths document for a piece.
func (r *Repo) SaveWindowPaths(ctx context.Context, doc WindowDoc) error {
	_, _ = r.Paths.DeleteOne(ctx, bson.M{"piece": doc.Piece})
	_, err := r.Paths.InsertOne(ctx, doc)
	return err
}
