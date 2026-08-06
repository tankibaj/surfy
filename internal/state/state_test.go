package state

import (
	"fmt"
	"strings"
	"testing"
)

func TestRegistryProjectLimit(t *testing.T) {
	t.Setenv("SERVE_THEM_STATE_DIR", t.TempDir())
	e := Update(func(r *Registry) error {
		for i := 0; i < 257; i++ {
			id := fmt.Sprint(i)
			r.Projects[id] = &Project{ID: id, Name: "p" + id}
		}
		return nil
	})
	if e == nil || !strings.Contains(e.Error(), "limit") {
		t.Fatalf("expected registry limit, got %v", e)
	}
}
