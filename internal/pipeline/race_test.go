package pipeline

import (
	"sync"
	"testing"

	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/model"
)

// TestConcurrentReconcileIsSafe. The HTTP server can serve a run while a batch
// is streaming, and a caller has every reason to expect a reconciliation to be
// a pure function of its inputs.
func TestConcurrentReconcileIsSafe(t *testing.T) {
	spec := generate.DefaultSpec()
	spec.Settlements = 12
	ds := generate.Generate(spec)
	eng := New(ds, DefaultConfig())

	seq := make([]string, len(ds.Credits))
	for i, c := range ds.Credits {
		seq[i] = string(eng.Reconcile(c).Status)
	}

	for attempt := 0; attempt < 5; attempt++ {
		got := make([]string, len(ds.Credits))
		var wg sync.WaitGroup
		for i, c := range ds.Credits {
			wg.Add(1)
			go func(i int, c model.BankCredit) { defer wg.Done(); got[i] = string(eng.Reconcile(c).Status) }(i, c)
		}
		wg.Wait()
		for i := range seq {
			if got[i] != seq[i] {
				t.Fatalf("attempt %d, settlement %d: concurrent %s, sequential %s", attempt, i, got[i], seq[i])
			}
		}
	}
}
