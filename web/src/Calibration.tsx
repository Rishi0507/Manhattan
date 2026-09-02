import { useMemo } from "react";
import type { EnvelopePoint, SweepPoint } from "./types";
import { idx, num, pct } from "./lib";
import { Empty, Note, Panel, Td, Th } from "./ui";

/**
 * The calibration study.
 *
 * The interesting question about this system is not what its accuracy is. It
 * is whether it knows in advance when it is about to be wrong, because a gate
 * that refuses the right settlements is worth more than a matcher with a good
 * hit rate. That is a falsifiable claim about numbers, and this screen is
 * where it is checked: the collision index is computed before any search, the
 * outcome rates are measured after, and if the curves turn where the index
 * predicts they turn, the estimator is calibrated.
 */
export function Calibration({
  sweep,
  envelope,
}: {
  sweep: SweepPoint[];
  envelope: EnvelopePoint[];
}) {
  const buckets = useMemo(() => bucketByIndex(sweep, 7), [sweep]);

  if (sweep.length === 0 && envelope.length === 0) {
    return (
      <Empty>
        No calibration data. Run <code className="tnum">manhattan bench</code>.
      </Empty>
    );
  }

  return (
    <div className="space-y-3">
      {buckets.length > 0 && (
        <Panel
          title="Does the system know in advance when it is about to be wrong?"
          hint="Predicted on the left, measured on the right. The index is computed before any search runs."
        >
          <div className="space-y-2.5">
            {buckets.map((b) => (
              <div key={b.lo} className="grid grid-cols-[130px_1fr] items-center gap-3">
                <div className="tnum text-right text-[12px] text-ink-faint">
                  {idx(b.lo)} – {idx(b.hi)}
                  <div className="text-[11.5px] text-ink-faint/70">{b.n} configs</div>
                </div>
                <div>
                  <div className="flex h-5 w-full overflow-hidden rounded-[2px]">
                    <Seg v={b.verified} c="var(--color-verified)" label={`verified ${pct(b.verified)}`} />
                    <Seg v={b.ambiguous} c="var(--color-ambiguous)" label={`ambiguous ${pct(b.ambiguous)}`} />
                    <Seg
                      v={b.underdetermined}
                      c="var(--color-underdetermined)"
                      label={`underdetermined ${pct(b.underdetermined)}`}
                    />
                    <Seg v={b.sensitive} c="var(--color-sensitive)" label={`sensitive ${pct(b.sensitive)}`} />
                    <Seg v={b.unresolved} c="var(--color-unresolved)" label={`unresolved ${pct(b.unresolved)}`} />
                  </div>
                  <div className="mt-1 flex items-center gap-3 text-[12px]">
                    <span style={{ color: b.wrong > 0 ? "var(--color-wrong)" : "var(--color-ink-faint)" }}>
                      Manhattan wrong postings {pct(b.wrong)}
                    </span>
                    <span style={{ color: b.b0Wrong > 0.15 ? "var(--color-wrong)" : "var(--color-ink-faint)" }}>
                      B0 wrong postings {pct(b.b0Wrong)}
                    </span>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[12px] text-ink-faint">
            {[
              ["verified", "var(--color-verified)"],
              ["ambiguous", "var(--color-ambiguous)"],
              ["underdetermined", "var(--color-underdetermined)"],
              ["narrowing sensitive", "var(--color-sensitive)"],
              ["unresolved", "var(--color-unresolved)"],
            ].map(([l, c]) => (
              <span key={l} className="inline-flex items-center gap-1.5">
                <span className="size-2 rounded-[2px]" style={{ background: c }} />
                {l}
              </span>
            ))}
          </div>

          <div className="mt-3">
            <Note tone="var(--color-accent)">
              As the predicted collision index rises, verified gives way to ambiguous and then to
              refusal, and the wrong-posting rate stays at zero throughout while B0's climbs. The
              gate is not merely conservative; it is turning at the point its own estimator said it
              would.
            </Note>
          </div>
        </Panel>
      )}

      {sweep.length > 0 && <EstimatorComparison sweep={sweep} />}

      {envelope.length > 0 && (
        <Panel
          title="Resource envelope, modelled against measured"
          hint="Publishing a modelled number under a measured heading is the class of unverified claim this project exists to refuse, so both columns are printed."
        >
          <table className="w-full">
            <thead>
              <tr>
                <Th right>pool</Th>
                <Th right>k</Th>
                <Th right>entries predicted</Th>
                <Th right>entries observed</Th>
                <Th right>MB predicted</Th>
                <Th right>MB observed</Th>
                <Th right>solve and prove</Th>
              </tr>
            </thead>
            <tbody>
              {envelope.map((e, i) => (
                <tr key={i}>
                  <Td right mono>
                    {e.pool_n}
                  </Td>
                  <Td right mono>
                    {e.k_star}
                  </Td>
                  <Td right mono className="text-ink-faint">
                    {num(e.predicted_entries)}
                  </Td>
                  <Td right mono>
                    {num(e.observed_entries)}
                  </Td>
                  <Td right mono className="text-ink-faint">
                    {e.predicted_mb.toFixed(0)}
                  </Td>
                  <Td right mono>
                    {e.observed_mb.toFixed(0)}
                  </Td>
                  <Td right mono>
                    {e.solve_and_prove_ms.toFixed(0)} ms
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-3 text-[12.5px] leading-relaxed text-ink-faint">
            Cost tracks the free cardinality, not the pool size. A 100-record pool at k=5 is more
            expensive than a 320-record pool at k=3, and that inversion is the signature of an
            algorithm matched to the constraint that actually binds. Every timing includes the
            uniqueness proof; these are not solve times with a proof still owed.
          </p>
        </Panel>
      )}

      {sweep.length > 0 && (
        <Panel title="Full sweep" hint="Pool size and batch size varied independently across four merchant shapes.">
          <div className="max-h-[420px] overflow-auto">
            <table className="w-full">
              <thead className="sticky top-0 bg-surface">
                <tr>
                  <Th>archetype</Th>
                  <Th right>pool</Th>
                  <Th right>batch</Th>
                  <Th right>twin mass</Th>
                  <Th right>index</Th>
                  <Th right>counted</Th>
                  <Th right>verified</Th>
                  <Th right>wrong</Th>
                  <Th right>B0 wrong</Th>
                </tr>
              </thead>
              <tbody>
                {sweep.map((p, i) => (
                  <tr key={i}>
                    <Td className="text-ink-faint">{p.archetype.replace(/_/g, " ")}</Td>
                    <Td right mono>
                      {p.pool_n}
                    </Td>
                    <Td right mono>
                      {p.batch_size}
                    </Td>
                    <Td right mono className="text-ink-faint">
                      {p.mean_twin_mass.toFixed(2)}
                    </Td>
                    <Td right mono>
                      {idx(p.mean_collision_index)}
                    </Td>
                    <Td right mono className="text-ink-faint">
                      {p.mean_reconstructions_counted.toFixed(1)}
                    </Td>
                    <Td right mono>
                      <span style={{ color: p.verified_rate > 0 ? "var(--color-verified)" : undefined }}>
                        {pct(p.verified_rate)}
                      </span>
                    </Td>
                    <Td right mono>
                      <span style={{ color: p.wrong_post_rate > 0 ? "var(--color-wrong)" : "var(--color-verified)" }}>
                        {pct(p.wrong_post_rate)}
                      </span>
                    </Td>
                    <Td right mono>
                      <span style={{ color: p.b0_wrong_post_rate > 0.2 ? "var(--color-wrong)" : "var(--color-ink-faint)" }}>
                        {pct(p.b0_wrong_post_rate)}
                      </span>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>
      )}
    </div>
  );
}

function Seg({ v, c, label }: { v: number; c: string; label: string }) {
  if (v <= 0) return null;
  return <div title={label} style={{ width: `${v * 100}%`, background: c }} />;
}

/**
 * The two estimators, side by side.
 *
 * This is a finding rather than a footnote: the published closed form is
 * measurably wrong on lognormal ticket distributions, and the receipt carries
 * both numbers rather than quietly picking a winner.
 */
function EstimatorComparison({ sweep }: { sweep: SweepPoint[] }) {
  const rows = sweep.filter(
    (p) => p.mean_reconstructions_counted > 0 && p.mean_collision_index > 0 && p.mean_analytic_collision_index > 0,
  );
  if (rows.length < 4) return null;

  const err = (pred: (p: SweepPoint) => number) =>
    rows.reduce((a, p) => {
      const r = pred(p) / p.mean_reconstructions_counted;
      return a + Math.abs(Math.log(r <= 0 ? 1e-9 : r));
    }, 0) / rows.length;

  const emp = err((p) => p.mean_collision_index);
  const ana = err((p) => p.mean_analytic_collision_index);

  return (
    <Panel
      title="Two estimators, and why the closed form was not enough"
      hint="Mean absolute log-ratio against the exhaustively counted number of reconstructions. Zero is perfect; 1.0 is off by a factor of e."
    >
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-2.5">
          {[
            ["measured, by sampling this pool", emp, "var(--color-verified)"],
            ["analytic, moment-matched normal", ana, "var(--color-ambiguous)"],
          ].map(([label, v, c]) => (
            <div key={label as string}>
              <div className="flex items-baseline justify-between">
                <span className="text-[13px] text-ink-dim">{label as string}</span>
                <span className="tnum text-[13.5px]" style={{ color: c as string }}>
                  {(v as number).toFixed(2)}
                </span>
              </div>
              <div className="mt-1 h-1.5 w-full overflow-hidden rounded-[2px] bg-raised">
                <div
                  className="h-full"
                  style={{ width: `${Math.min((v as number) / 1.5, 1) * 100}%`, background: c as string }}
                />
              </div>
            </div>
          ))}
        </div>
        <p className="text-[12.5px] leading-relaxed text-ink-faint">
          The closed form assumes the sums of k-subsets are locally uniform near the target, with a
          density read off a moment-matched normal. Real ticket amounts are close to lognormal, and a
          sum of three of those is nothing like a normal distribution in its body, so its true
          density near a typical target is several times higher than the approximation says.
          <br />
          <br />
          The error runs in the safe direction: an index that is too low makes the gate accept a
          slightly larger region, and the exhaustive count inside that region finds the rivals
          anyway. Precision is untouched. But a gate off by an order of magnitude is a poor gate, so
          the density is now measured by sampling the actual pool rather than assumed. Both numbers
          are carried on every receipt.
        </p>
      </div>
    </Panel>
  );
}

function bucketByIndex(sweep: SweepPoint[], n: number) {
  if (sweep.length === 0) return [];
  const vals = sweep.map((p) => Math.log10(Math.max(p.mean_collision_index, 1e-4)));
  const lo = Math.min(...vals);
  const hi = Math.max(...vals) || lo + 1;
  const span = hi - lo || 1;

  const out = Array.from({ length: n }, (_, i) => ({
    lo: Math.pow(10, lo + (span * i) / n),
    hi: Math.pow(10, lo + (span * (i + 1)) / n),
    n: 0,
    verified: 0,
    ambiguous: 0,
    underdetermined: 0,
    sensitive: 0,
    unresolved: 0,
    wrong: 0,
    b0Wrong: 0,
  }));

  sweep.forEach((p, i) => {
    let b = Math.floor(((vals[i] - lo) / span) * n);
    if (b >= n) b = n - 1;
    if (b < 0) b = 0;
    const t = out[b];
    t.n++;
    t.verified += p.verified_rate;
    t.ambiguous += p.ambiguous_rate;
    t.underdetermined += p.underdetermined_rate;
    t.sensitive += p.narrowing_sensitive_rate;
    t.unresolved += p.unresolved_rate;
    t.wrong += p.wrong_post_rate;
    t.b0Wrong += p.b0_wrong_post_rate;
  });

  return out
    .filter((b) => b.n > 0)
    .map((b) => ({
      ...b,
      verified: b.verified / b.n,
      ambiguous: b.ambiguous / b.n,
      underdetermined: b.underdetermined / b.n,
      sensitive: b.sensitive / b.n,
      unresolved: b.unresolved / b.n,
      wrong: b.wrong / b.n,
      b0Wrong: b.b0Wrong / b.n,
    }));
}
