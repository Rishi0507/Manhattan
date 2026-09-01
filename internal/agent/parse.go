package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/model"
)

// Narration is the typed reading of a bank statement line.
//
// Every field carries a provenance span into the original text, so a
// reconciliation can point at the characters a claim came from. A parser that
// returns values without provenance is asking to be trusted; one that returns
// spans can be checked.
type Narration struct {
	BankReference  string `json:"bank_reference"`
	Channel        string `json:"channel"`
	Counterparty   string `json:"counterparty"`
	IsSettlement   bool   `json:"is_settlement"`
	Direction      string `json:"direction"`
	Confident      bool   `json:"confident"`
	ProvenanceSpan string `json:"provenance_span"`
}

// Parser reads free-text bank narrations into typed fields.
//
// This is the agent's first job and it is a genuine one. Narration formats
// differ by bank, by channel, by sponsor and by decade; the same settlement
// arrives as "NEFT-RAZORPAY SOFTWARE PVT LTD-UTR3491-CR" from one bank and
// "RTGS CR RAZORPAYSOFTWAREPVTLTD REF 884120 NET STLMNT" from another. A
// regex set covers whichever formats its author had seen and rots quietly
// against the rest, and the rot shows up as settlements silently attributed
// to the wrong merchant.
//
// What keeps it safe is not that the model is good at it. It is that the
// output is a typed schema whose validation failures become exceptions rather
// than guesses, and that nothing the parser produces can move money on its
// own: it selects which pool to search, and the search still has to close an
// integer identity uniquely before anything posts.
type Parser struct {
	Provider llm.Provider
}

// NewParser returns a parser over the given provider.
func NewParser(p llm.Provider) *Parser { return &Parser{Provider: p} }

const parseSystem = `You read a single line of free text from an Indian bank statement and return
typed fields. The line describes a credit that may or may not be a payment gateway
settlement.

Return only what the text supports. If a field is not present, return an empty string
rather than inferring one, and set confident to false. A wrong bank reference is worse
than a missing one, because a missing one routes the credit to review while a wrong one
routes it confidently to the wrong merchant.

Formats vary by bank and by channel. Common Indian shapes include:
  NEFT-<COUNTERPARTY>-UTR<REF>-CR
  IMPS/P2A/<REF>/<COUNTERPARTY>/SETTLEMENT
  RTGS CR <COUNTERPARTYNOSPACES> REF <REF> NET STLMNT
  UPI/CREDIT/<REF>/<COUNTERPARTY>/SETTLE/NA

The reference is the UTR, RRN or bank reference number: the identifier the sponsor bank
assigned to the transfer. Note that the UTR is issued by the banking partner and not by
the payment gateway, so it is not the gateway's own settlement id and cannot be used as
a join key against gateway records.

provenance_span is the character offset range in the input where the reference was
found, written as "start:end". It lets a reader check the reading against the text.`

// Parse reads one narration.
func (p *Parser) Parse(ctx context.Context, credit model.BankCredit) (Narration, llm.Usage, error) {
	res, err := p.Provider.Structured(ctx, llm.Request{
		Role:   llm.RoleParse,
		System: parseSystem,
		// The volatile half of the request is one line of text, whatever the
		// size of the candidate pool. That is what keeps per-settlement model
		// spend flat as merchants grow, and it is the whole of the cost
		// argument against putting the pool in the context window.
		User:       "STATEMENT LINE: " + credit.Narration,
		SchemaName: "read_narration",
		SchemaDesc: "Extract typed fields from one bank statement narration.",
		Schema:     narrationSchema(),
		MaxTokens:  512,
	})
	if err != nil {
		return Narration{}, llm.Usage{}, err
	}
	var n Narration
	if err := res.Into(&n); err != nil {
		return Narration{}, res.Usage, err
	}
	return n, res.Usage, nil
}

// Reconcilable reports whether the parsed narration supports treating this
// credit as a settlement to reconcile, and says why if not.
//
// A parse that is not confident does not stop the pipeline; it changes what
// the receipt claims. The credit is still reconstructed from the pool, and
// the receipt records that the counterparty was read with low confidence, so
// an analyst reviewing a posting can see that one of its premises was soft.
func (n Narration) Reconcilable() (bool, string) {
	if !n.IsSettlement {
		return false, "the narration does not describe a settlement credit"
	}
	if strings.EqualFold(n.Direction, "debit") {
		return false, "the narration describes a debit, not a credit"
	}
	if n.BankReference == "" {
		return true, "no bank reference could be read from the narration, so the credit is " +
			"identified by amount, date and counterparty alone"
	}
	return true, ""
}

func narrationSchema() map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bank_reference": str("The UTR, RRN or bank reference number, or an empty string if none is present."),
			"channel": map[string]any{
				"type":        "string",
				"enum":        []string{"NEFT", "RTGS", "IMPS", "UPI", "ACH", "NACH", "UNKNOWN"},
				"description": "The transfer rail.",
			},
			"counterparty":  str("The paying entity as written, normalised to spaced upper case, or empty."),
			"is_settlement": map[string]any{"type": "boolean", "description": "Whether this line describes a gateway settlement payout."},
			"direction": map[string]any{
				"type": "string", "enum": []string{"credit", "debit", "unknown"},
			},
			"confident":       map[string]any{"type": "boolean", "description": "False if any field was inferred rather than read."},
			"provenance_span": str("Character offsets of the reference in the input, as start:end, or empty."),
		},
		"required": []string{
			"bank_reference", "channel", "counterparty",
			"is_settlement", "direction", "confident", "provenance_span",
		},
	}
}

// String renders a parsed narration for a receipt.
func (n Narration) String() string {
	return fmt.Sprintf("%s via %s from %s", n.BankReference, n.Channel, n.Counterparty)
}
