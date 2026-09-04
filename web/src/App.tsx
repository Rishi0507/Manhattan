import { useCallback, useEffect, useMemo, useState } from "react";
import type { CaseOutcome, EnvelopePoint, Receipt, Summary, SweepPoint } from "./types";
import { api, cls } from "./lib";
import { Tabs } from "./ui";
import { Landing } from "./Landing";
import { Close } from "./Close";
import { HeadToHead } from "./HeadToHead";
import { Run } from "./Run";
import { Cases } from "./Cases";
import { Exceptions } from "./Exceptions";
import { Calibration } from "./Calibration";
import { Ask } from "./Ask";
import { ReceiptView } from "./ReceiptView";

type Tab = "close" | "hook" | "run" | "cases" | "exceptions" | "calibration" | "ask";

/**
 * The shell.
 *
 * A landing page comes first and the six tabs stay out of sight until someone
 * enters. Six labelled destinations on first paint is the fastest way to make
 * a viewer feel behind, and the argument this project makes needs about
 * fifteen seconds of undivided attention before any of the destinations mean
 * anything.
 */
export default function App() {
  const [entered, setEntered] = useState(false);
  // The close is the landing tab.
  //
  // Every other view answers "what happened to these settlements". The close
  // answers "what do I do on Monday", which is the question an operations lead
  // actually arrives with, and it is the one view that reads across the period
  // rather than within one settlement.
  const [tab, setTab] = useState<Tab>("close");
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [cases, setCases] = useState<CaseOutcome[]>([]);
  const [sweep, setSweep] = useState<SweepPoint[]>([]);
  const [envelope, setEnvelope] = useState<EnvelopePoint[]>([]);
  const [open, setOpen] = useState<Receipt | null>(null);

  // The drawer outlives its own dismissal by one animation.
  //
  // Unmounting on click makes a panel that slid in simply cease to exist,
  // which reads as a glitch rather than as a close: the eye tracked it
  // arriving and expects it to leave the same way. So `closing` keeps the last
  // receipt mounted for the length of the exit, and `shown` is what actually
  // renders. The timer is cleared if something reopens in the meantime, which
  // is what stops a fast click-close-click from leaving a ghost behind.
  const [closing, setClosing] = useState(false);
  const [leaving, setLeaving] = useState<Receipt | null>(null);
  const shown = open ?? (closing ? leaving : null);

  const dismiss = useCallback(() => {
    setOpen((cur) => {
      if (cur) {
        setLeaving(cur);
        setClosing(true);
      }
      return null;
    });
  }, []);

  useEffect(() => {
    if (!closing) return;
    const t = window.setTimeout(() => {
      setClosing(false);
      setLeaving(null);
    }, 140);
    return () => window.clearTimeout(t);
  }, [closing]);

  useEffect(() => {
    if (open) setClosing(false);
  }, [open]);

  // Escape closes it, with the same animation as the button.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") dismiss();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, dismiss]);
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
        setEntered(true);
        setTab("run");
      } else if (ev.type === "settlement" && ev.receipt) {
        const rec = ev.receipt;
        setReceipts((prev) =>
          prev.some((p) => p.settlement_ref === rec.settlement_ref) ? prev : [...prev, rec],
        );
        setProgress((p) => (p ? { done: ev.done, total: ev.total } : p));
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

  const exceptionCount = useMemo(
    () => receipts.filter((r) => r.status !== "VERIFIED").length,
    [receipts],
  );

  const enter = useCallback((t: Tab) => {
    setEntered(true);
    setTab(t);
  }, []);

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-20 border-b border-line bg-ground/92 backdrop-blur">
        <div className="mx-auto max-w-[1240px] px-4 sm:px-6">
          <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3">
            <button
              onClick={() => setEntered(false)}
              className="flex items-baseline gap-3 text-left"
              title="back to the overview"
            >
              {/* Aligned on the text baseline rather than centred.
                  An image's baseline is its bottom edge, and this mark's bottom
                  edge IS a baseline rule, so the rule lands exactly on the
                  wordmark's baseline, then nudged down a few pixels so the mark
                  sits optically centred against the height of the wordmark
                  block rather than hanging off the top of it. Centring it
                  geometrically instead pushed the rule below the text and read
                  as an underline. */}
              <img
                src="/logo.png"
                alt=""
                className="h-[19px] w-auto shrink-0 translate-y-[3px] sm:h-[22px] sm:translate-y-[4px]"
              />
              <span className="display text-[27px] leading-none font-semibold tracking-[-0.012em] text-ink sm:text-[31px]">
                Manhattan
              </span>
              <span className="hidden text-[13.5px] text-ink-faint sm:inline">
                proves settlements instead of guessing them
              </span>
            </button>

            <div className="flex items-center gap-3">
              {provider && (
                <span
                  className="tnum hidden text-[12px] text-ink-faint sm:inline"
                  title="which language model backs this run"
                >
                  {provider}
                </span>
              )}
              <button
                onClick={() => void startRun()}
                disabled={streaming}
                className="rounded-md border border-line-strong px-3 py-1.5 text-[13px] text-ink-dim transition-colors hover:border-accent hover:text-accent disabled:opacity-50"
              >
                {streaming ? "running" : "run a batch"}
              </button>
            </div>
          </div>

          {entered && (
            <Tabs<Tab>
              active={tab}
              onChange={setTab}
              tabs={[
                { id: "close", label: "The close" },
                { id: "hook", label: "Head to head" },
                { id: "run", label: "Run", badge: receipts.length || undefined },
                { id: "cases", label: "Cases", badge: cases.length || undefined },
                { id: "exceptions", label: "Exceptions", badge: exceptionCount || undefined },
                { id: "calibration", label: "Calibration" },
                { id: "ask", label: "Ask" },
              ]}
            />
          )}
        </div>
      </header>

      {loadError && (
        <p
          className="mx-auto mt-4 max-w-[1240px] rounded-md border border-line px-4 py-2.5 text-[13px]"
          style={{ color: "var(--color-wrong)" }}
        >
          {loadError}
        </p>
      )}

      {!entered ? (
        <div className="view-in">
          <Landing summary={summary} cases={cases} onEnter={enter} />
        </div>
      ) : (
        <main key={tab} className="view-in mx-auto max-w-[1240px] px-4 py-4 sm:px-6 sm:py-5">
          {tab === "close" && <Close summary={summary} />}
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
      )}

      {/* The evidence object, in a drawer rather than a new page, so the
          context a viewer was reading stays behind it. */}
      {shown && (
        <div
          className={cls(
            "fixed inset-0 z-30 flex justify-end bg-[#2c2318]/25 backdrop-blur-[1px]",
            closing && "scrim-out",
          )}
          onClick={dismiss}
          role="dialog"
          aria-modal="true"
        >
          <div
            className={cls(
              "h-full w-full max-w-[980px] overflow-y-auto border-line-strong bg-ground sm:border-l",
              "shadow-[-8px_0_28px_rgba(44,35,24,0.12)]",
              closing ? "panel-out" : "panel-in",
            )}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="sticky top-0 z-10 flex items-center justify-between border-b border-line bg-ground/95 px-4 py-2 backdrop-blur">
              <span className="lbl">evidence object</span>
              <button
                onClick={dismiss}
                className="rounded-md border border-line px-2.5 py-1 text-[12.5px] text-ink-faint transition-colors hover:border-ink-faint hover:text-ink-dim"
              >
                close
              </button>
            </div>
            <div className="p-3 sm:p-4">
              <ReceiptView r={shown} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
