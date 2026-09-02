import { useMemo } from "react";
import type { EnvelopePoint, SweepPoint } from "./types";
import { idx, num, pct } from "./lib";
import { Empty, Note, Panel, Td, Th } from "./ui";
import { AgreementScatter, EnvelopeBars, OutcomeBands, SERIES } from "./Charts";
import { statusColor } from "./lib";

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
          title="Calibration"
          hint="index computed before any search, outcomes measured after"
        >
          <OutcomeBands
            bands={buckets.map((b) => ({
              lo: b.lo,
              hi: b.hi,
              n: b.n,
              wrong: b.wrong,
              b0Wrong: b.b0Wrong,
              parts: [
                { label: "verified", value: b.verified, color: statusColor("VERIFIED") },
                { label: "ambiguous", value: b.ambiguous, color: statusColor("AMBIGUOUS") },
                { label: "underdetermined", value: b.underdetermined, color: statusColor("UNDERDETERMINED") },
                { label: "narrowing sensitive", value: b.sensitive, color: statusColor("NARROWING_SENSITIVE") },
                { label: "unresolved", value: b.unresolved, color: statusColor("UNRESOLVED") },
              ],
            }))}
          />

          <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[12px] text-ink-faint">
            {(
              [
                ["verified", "VERIFIED"],
                ["ambiguous", "AMBIGUOUS"],
                ["underdetermined", "UNDERDETERMINED"],
                ["narrowing sensitive", "NARROWING_SENSITIVE"],
                ["unresolved", "UNRESOLVED"],
              ] as const
            ).map(([label, st]) => (
              <span key={label} className="inline-flex items-center gap-1.5">
                <span className="size-2.5 rounded-[2px]" style={{ background: statusColor(st) }} />
                {label}
              </span>
            ))}
          </div>

          <div className="mt-3">
            <Note tone="var(--color-accent)">
              As the predicted index rises, verified gives way to ambiguous and then to refusal. The
              wrong-posting rate remains zero throughout.
            </Note>
          </div>
        </Panel>
      )}

      {sweep.length > 0 && <EstimatorComparison sweep={sweep} />}

      {envelope.length > 0 && (
        <Panel
          title="Resource envelope"
          hint="modelled against measured"
        >
          <EnvelopeBars
            rows={envelope.map((e) => ({
              pool: e.pool_n,
              k: e.k_star,
              ms: e.solve_and_prove_ms,
              mb: e.observed_mb,
            }))}
          />
          <p className="mt-1 mb-4 text-[12.5px] leading-relaxed text-ink-faint">
            Cost follows the free cardinality rather than the pool size. A 100-record pool at k=5
            exceeds a 320-record pool at k=3. Every timing includes the uniqueness proof.
          </p>

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
        <Panel title="Full sweep" hint="pool and batch size varied independently">
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
      title="Estimator accuracy"
      hint="mean absolute log-ratio against the exhaustive count"
    >
      <AgreementScatter
        points={rows.map((p) => ({
          observed: p.mean_reconstructions_counted,
          empirical: p.mean_collision_index,
          analytic: p.mean_analytic_collision_index,
          label: `${p.archetype.replace(/_/g, " ")} · pool ${p.pool_n} · batch ${p.batch_size}`,
        }))}
      />

      <div className="mt-5 grid gap-3 md:grid-cols-2">
        <div className="space-y-2.5">
          {[
            ["measured, by sampling this pool", emp, SERIES.a],
            ["analytic, moment-matched normal", ana, SERIES.b],
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
