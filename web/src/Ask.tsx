import { useState } from "react";
import type { Answer } from "./types";
import { api } from "./lib";
import { Note, Panel } from "./ui";

/**
 * The question-answering agent, over the receipt store.
 *
 * The evidence object is already a complete structured derivation, and a
 * finance team's real questions are answerable from its fields. Nobody wants
 * to answer them by reading JSON, so this exists. It is constrained the same
 * way everything else here is: every claim carries a receipt id and a field
 * path, nothing is inferred beyond what a receipt records, and it cannot
 * re-run a reconciliation or change a status.
 *
 * "I do not have a receipt that says that" is a valid and expected answer.
 */
const SUGGESTIONS = [
  "Why didn't the largest exception post, and what would make it?",
  "Which constraint dropped the most records across this run?",
  "Which of our merchants are hardest to reconcile, and why?",
  "Show me every settlement where the fee check was circular.",
  "What is the exception backlog costing us?",
];

export function Ask() {
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [history, setHistory] = useState<{ q: string; a: Answer }[]>([]);

  async function ask(question: string) {
    const text = question.trim();
    if (!text || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const a = await api<Answer>("/api/ask", {
        method: "POST",
        body: JSON.stringify({ question: text }),
      });
      setHistory((h) => [{ q: text, a }, ...h]);
      setQ("");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <Panel
        title="Ask the receipt store"
        hint="Answers are grounded in stored evidence and nothing else. Every claim carries a receipt id and a field path."
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void ask(q);
          }}
          className="flex gap-2"
        >
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Why didn't 1042 post, and what would make it?"
            className="flex-1 rounded-md border border-line bg-ground px-3 py-2 text-[12.5px] text-ink placeholder:text-ink-faint focus:border-accent focus:outline-none"
          />
          <button
            type="submit"
            disabled={busy || !q.trim()}
            className="rounded border border-accent bg-accent/10 px-4 py-2 text-[12.5px] text-accent transition-colors hover:bg-accent/20 disabled:cursor-not-allowed disabled:border-line disabled:bg-transparent disabled:text-ink-faint"
          >
            {busy ? "reading receipts" : "ask"}
          </button>
        </form>

        <div className="mt-3 flex flex-wrap gap-1.5">
          {SUGGESTIONS.map((s) => (
            <button
              key={s}
              onClick={() => void ask(s)}
              disabled={busy}
              className="rounded-[3px] border border-line px-2.5 py-1 text-[11px] text-ink-faint transition-colors hover:border-ink-faint hover:text-ink-dim disabled:opacity-50"
            >
              {s}
            </button>
          ))}
        </div>

        {err && (
          <p className="mt-3 text-[11.5px]" style={{ color: "var(--color-wrong)" }}>
            {err}
          </p>
        )}
      </Panel>

      {history.map((h, i) => (
        <Panel key={i} title={h.q}>
          <p className="text-[12.5px] leading-relaxed whitespace-pre-wrap text-ink">{h.a.answer}</p>

          {!h.a.answerable && (
            <div className="mt-3">
              <Note tone="var(--color-ambiguous)">
                The receipts do not contain what this question asks for. That is a valid answer: the
                agent reads stored evidence and declines rather than inferring.
              </Note>
            </div>
          )}

          {h.a.citations?.length > 0 && (
            <div className="mt-3 border-t border-line-soft pt-2.5">
              <div className="lbl mb-1.5">grounded in</div>
              <div className="space-y-1">
                {h.a.citations.map((c, j) => (
                  <div key={j} className="tnum flex flex-wrap items-baseline gap-x-3 text-[11px]">
                    <span className="text-ink-dim">{c.receipt_id.replace("bank_credit_", "")}</span>
                    <span className="text-accent">{c.field}</span>
                    {c.value && <span className="text-ink-faint">= {c.value}</span>}
                  </div>
                ))}
              </div>
            </div>
          )}

          {h.a.retrieved && h.a.retrieved.length > 0 && (
            <p className="tnum mt-3 text-[11px] text-ink-faint">
              {h.a.retrieved.length} receipt{h.a.retrieved.length === 1 ? "" : "s"} were put in front
              of the model; {h.a.citations?.length ?? 0} were cited.
            </p>
          )}
        </Panel>
      ))}

      {history.length === 0 && (
        <Note>
          This is what an auditable evidence object is for. The question-answering agent is not a
          bolt-on: it is the reason the verifier writes down its reasoning rather than only its
          conclusion.
        </Note>
      )}
    </div>
  );
}
