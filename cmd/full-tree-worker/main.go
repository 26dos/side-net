package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	mgo "side-net/pkg/mongo"
	"side-net/pkg/tree"
	"side-net/pkg/util"
)

// full-tree-worker: loops success results, downloads piece bytes, builds Filecoin-compatible
// Poseidon/Fr32 full tree (via Rust CLI), stores window paths in Mongo, and marks record failed if any step fails.
func main() {
	ctx := context.Background()
	repo, err := mgo.New(ctx, must("MONGO_URI"), env("MONGO_DB","fil"))
	check(err)
	store := &tree.Store{Repo: repo}

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			if err := processOne(ctx, repo, store); err != nil {
				fmt.Fprintln(os.Stderr, "process:", err)
				time.Sleep(2 * time.Second)
			}
		}
	}
}

func processOne(ctx context.Context, repo *mgo.Repo, store *tree.Store) error {
	doc, err := repo.FindOneSuccess(ctx)
	if err != nil { return nil } // nothing to process now

	ip, port, ok := util.FirstIP4TCP(doc.Task.Provider.Multiaddrs)
	if !ok {
		_ = repo.MarkFailed(ctx, doc.ID, "no_ip_port", "no ip4/tcp in multiaddrs")
		return fmt.Errorf("no ip/tcp in multiaddrs")
	}

	url := fmt.Sprintf("http://%s:%s/piece/%s", ip, port, doc.Task.Content.CID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil && resp.Body != nil { resp.Body.Close() }
		_ = repo.MarkFailed(ctx, doc.ID, "download_error", fmt.Sprintf("GET %s -> %v", url, err))
		return fmt.Errorf("download error")
	}
	defer resp.Body.Close()

	pr, pw := io.Pipe()
	go func(){ defer pw.Close(); _, _ = io.Copy(pw, resp.Body) }()

	if err := store.BuildWindowsAndSave(ctx, doc.Task.Content.CID, pr); err != nil {
		_ = repo.MarkFailed(ctx, doc.ID, "build_error", err.Error())
		return err
	}
	return nil
}

func must(k string) string { v:=os.Getenv(k); if v=="" { panic("missing "+k) }; return v }
func env(k, d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
func check(err error){ if err!=nil { panic(err) } }
