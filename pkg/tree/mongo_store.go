package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"side-net/pkg/mongo"
)

// Store invokes the Rust CLI to build the Poseidon/Fr32 tree window paths and persists them.
type Store struct {
	Repo *mongo.Repo
}

// BuildWindowsAndSave runs `piece-tree-cli`, feeds raw bytes via stdin, parses JSON on stdout,
// and saves the exported window paths into Mongo.
func (s *Store) BuildWindowsAndSave(ctx context.Context, piece string, raw io.Reader) error {
	cmd := exec.CommandContext(ctx, "piece-tree-cli")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	go func() { defer stdin.Close(); _, _ = io.Copy(stdin, raw) }()

	var decoded map[string]any
	if err := json.NewDecoder(stdout).Decode(&decoded); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("decode: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("cli exit: %w", err)
	}

	doc := mongo.WindowDoc{
		Piece:     piece,
		BuiltAt:   time.Now().UTC(),
		HashAlgo:  getStr(decoded, "hash_algo"),
		Arity:     int(getFloat(decoded, "arity")),
		LeafSize:  int(getFloat(decoded, "leaf_size")),
		Root:      getStr(decoded, "root"),
		WindowSzB: int(getFloat(decoded, "window_size_bytes")),
		Paths:     getAnySlice(decoded, "window_paths"),
		Meta:      map[string]any{"impl": "poseidon-filecoin"},
	}
	return s.Repo.SaveWindowPaths(ctx, doc)
}

func getStr(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
func getFloat(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}
func getAnySlice(m map[string]any, k string) []any {
	if v, ok := m[k].([]any); ok {
		return v
	}
	return nil
}
