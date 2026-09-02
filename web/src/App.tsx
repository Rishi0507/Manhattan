import { useCallback, useEffect, useMemo, useState } from "react";
import type { CaseOutcome, EnvelopePoint, Receipt, Summary, SweepPoint } from "./types";
import { api, cls } from "./lib";
import { Tabs } from "./ui";
import { HeadToHead } from "./HeadToHead";
import { Run } from "./Run";
import { Cases } from "./Cases";
import { Exceptions } from "./Exceptions";
import { Calibration } from "./Calibration";
import { Ask } from "./Ask";
import { ReceiptView } from "./ReceiptView";

type Tab = "hook" | "run" | "cases" | "exceptions" | "calibration" | "ask";

export default function App() {
  const [tab, setTab] = useState<Tab>("hook");
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [cases, setCases] = useState<CaseOutcome[]>([]);
  const [sweep, setSweep] = useState<SweepPoint[]>([]);
  const [envelope, setEnvelope] = useState<EnvelopePoint[]>([]);
  const [open, setOpen] = useState<Receipt | null>(null);
  const [provider, setProvider] = useState<string>("");
  const [streaming, setStreaming] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [rs, run, cs, sw] = await Promise.all([
        api<Receipt[] | null>("/api/receipts"),
        api<{ summary: Summary | null; provider: string }>("/api/run"),
        api<CaseOutcome[] | null>("/api/cases"),
        api<{ points: SweepPoint[] | null; envelope: EnvelopePoint[] | null }>("/api/sweep"),
      ]);
      setReceipts(rs ?? []);
      setSummary(run.summary ?? null);
      setProvider(run.provider ?? "");
      setCases(cs ?? []);
      setSweep(sw.points ?? []);
      setEnvelope(sw.envelope ?? []);
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // The live stream. A run reads as an agent working rather than as a report
  // being displayed, which is most of the difference between the two.
  useEffect(() => {
    const es = new EventSource("/api/stream");
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data) as {
        type: string;
        receipt?: Receipt;
        done: number;
        total: number;
        summary?: Summary;
      };
      if (ev.type === "start") {
        setReceipts([]);
        setStreaming(true);
        setProgress({ done: 0, total: ev.total });
        setTab("run");
      } else if (ev.type === "settlement" && ev.receipt) {
        const rec = ev.receipt;
        setReceipts((prev) => (prev.some((p) => p.settlement_ref === rec.settlement_ref) ? prev : [...prev, rec]));
        setProgress({ done: ev.done, total: ev.total });
      } else if (ev.type === "done") {
        setStreaming(false);
        setProgress(null);
        if (ev.summary) setSummary(ev.summary);
      }
    };
    es.onerror = () => es.close();
    return () => es.close();
  }, []);

  const startRun = useCallback(async () => {
    try {
      await api("/api/run/start", { method: "POST", body: JSON.stringify({ settlements: 300 }) });
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const exceptionCount = useMemo(() => receipts.filter((r) => r.status !== "VERIFIED").length, [receipts]);

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-20 border-b border-line bg-ground/95 backdrop-blur">
        <div className="mx-auto max-w-[1400px] px-5">
          <div className="flex flex-wrap items-baseline justify-between gap-x-8 gap-y-2 py-3">
            <div className="flex items-baseline gap-3.5">
              <h1 className="text-[15px] font-semibold tracking-tight text-ink">Manhattan</h1>
              <p className="text-[12px] text-ink-faint">
                An agent that proves settlements instead of guessing them.
              </p>
            </div>
            <div className="flex items-center gap-3">
              {provider && (
                <span className="tnum text-[11px] text-ink-faint" title="which language model backs this run">
                  {provider}
                </span>
              )}
              <button
                onClick={() => void startRun()}
                disabled={streaming}
                className="rounded-md border border-line px-3 py-1.5 text-[11.5px] text-ink-dim transition-colors hover:border-accent hover:text-accent disabled:opacity-50"
              >
                {streaming ? "running" : "run a batch"}
              </button>
            </div>
          </div>

          <Tabs<Tab>
            active={tab}
            onChange={setTab}
            tabs={[
              { id: "hook", label: "Head to head" },
              { id: "run", label: "Run", badge: receipts.length || undefined },
              { id: "cases", label: "Adversarial cases", badge: cases.length || undefined },
              { id: "exceptions", label: "Exception queue", badge: exceptionCount || undefined },
              { id: "calibration", label: "Calibration" },
              { id: "ask", label: "Ask" },
            ]}
          />
        </div>
      </header>

      <main className="mx-auto max-w-[1400px] px-5 py-4">
        {loadError && (
          <p className="mb-3 rounded-md border border-line px-3.5 py-2.5 text-[12px]" style={{ color: "var(--color-wrong)" }}>
            {loadError}
          </p>
        )}

        {tab === "hook" && <HeadToHead cases={cases} />}
        {tab === "run" && (
          <Run
            receipts={receipts}
            summary={summary}
            onOpen={setOpen}
            streaming={streaming}
            progress={progress}
          />
        )}
        {tab === "cases" && <Cases cases={cases} onOpen={setOpen} />}
        {tab === "exceptions" && <Exceptions receipts={receipts} onOpen={setOpen} />}
        {tab === "calibration" && <Calibration sweep={sweep} envelope={envelope} />}
        {tab === "ask" && <Ask />}
      </main>

      {/* The evidence object, in a drawer rather than a new page, so the
          context a viewer was reading stays behind it. */}
      {open && (
        <div
          className="fixed inset-0 z-30 flex justify-end bg-[#16181d]/25 backdrop-blur-[1px]"
          onClick={() => setOpen(null)}
          role="dialog"
          aria-modal="true"
        >
          <div
            className={cls(
              "h-full w-full max-w-[980px] overflow-y-auto border-l border-line-strong bg-ground",
              "shadow-[-8px_0_24px_rgba(22,24,29,0.10)]",
            )}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="sticky top-0 z-10 flex items-center justify-between border-b border-line bg-ground/95 px-4 py-2 backdrop-blur">
              <span className="text-[12px] text-ink-faint">evidence object</span>
              <button
                onClick={() => setOpen(null)}
                className="rounded-md border border-line px-2.5 py-1 text-[11.5px] text-ink-faint transition-colors hover:border-ink-faint hover:text-ink-dim"
              >
                close
              </button>
            </div>
            <div className="p-4">
              <ReceiptView r={open} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
