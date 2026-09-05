package agent

import (
	"context"
	"os"
	"testing"

	"github.com/Rishi0507/manhattan/internal/llm"
)

// TestControllerSchemaAcceptedLive sends the controller's real schema to the
// live provider once.
//
// The batch absorbs a provider error as an exception the agent could not clear,
// so a schema the API rejects costs a full run to discover and looks like a
// weak model when it arrives. One call, with the actual schema rather than a
// simplified copy, is the cheap way to find out first.
func TestControllerSchemaAcceptedLive(t *testing.T) {
	if os.Getenv("MANHATTAN_LIVE_SMOKE") != "1" || os.Getenv("GROQ_API_KEY") == "" {
		t.Skip("set MANHATTAN_LIVE_SMOKE=1 and GROQ_API_KEY to spend a request on this")
	}
	p := llm.NewGroq(llm.DefaultGroqConfig())
	res, err := p.Structured(context.Background(), llm.Request{
		Role:       llm.RolePlan,
		SchemaName: "choose_action",
		System:     "You choose one reconciliation action. Fields that do not apply must be null.",
		User:       "The pool is too wide and nothing reconstructs. Choose one action.",
		Schema:     actionSchema(),
		MaxTokens:  1200,
	})
	if err != nil {
		t.Fatalf("controller schema rejected live: %v", err)
	}
	t.Logf("accepted: %s", res.JSON)
}
