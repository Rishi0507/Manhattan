import type { Receipt } from "./types";
import { constraintLabel, flagMeaning, idx, num, rupees, statusColor, statusMeaning } from "./lib";
import { Bar, Field, Flag, Note, Panel, StatusPill, Td, Th } from "./ui";

/**
 * The evidence object, rendered.
 *
 * This is the artifact the whole system exists to produce, so the screen is
 * organised the way the decision was actually made: what was excluded and
 * why, whether the question was answerable at all, what was searched, what
 * was found, what was ruled out, and what would be needed to do better.
 *
 * Nothing is hidden behind a disclosure. An audit trail that requires
 * clicking is one that does not get read.
 */
export function ReceiptView({ r }: { r: Receipt }) {
  const u = r.uniqueness;
  const f = r.feasibility;
  const probe = r.narrowing.neighbourhood_probe;
  const dropped = Object.entries(r.narrowing.dropped).sort((a, b) => b[1] - a[1]);
  const tone = statusColor(r.status);

  return (
    <div className="space-y-3">
      {/* Headline */}
      <div className="rounded-md border border-line bg-surface">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line-soft px-4 py-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <StatusPill status={r.status} />
              {r.flags.map((x) => (
                <Flag key={x} name={x} title={flagMeaning(x)} />
              ))}
            </div>
            <div className="tnum mt-2.5 text-[24px] leading-none text-ink">{rupees(r.target_paise)}</div>
            <div className="tnum mt-1.5 text-[11.5px] text-ink-faint">{r.settlement_ref}</div>
          </div>
          <div className="text-right text-[11.5px] text-ink-faint">
            <div className="tnum">{r.narration}</div>
            <div className="tnum mt-0.5">
              {r.merchant_name} · {r.merchant_archetype} · value date {r.value_date}
            </div>
            <div className="tnum mt-0.5">{r.data_mode.replace(/_/g, " ")}</div>
          </div>
        </div>

        <div className="space-y-2 px-4 py-3">
          <p className="text-[12.5px] leading-relaxed text-ink">{r.claim}</p>
          {r.note && <p className="text-[12px] leading-relaxed text-ink-faint">{r.note}</p>}
          <p className="pt-1 text-[11.5px] leading-relaxed text-ink-faint">{statusMeaning(r.status)}</p>
        </div>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        {/* Stage 2: narrowing */}
        <Panel
          title="Narrowing"
          hint="Every excluded record is logged with the rule that excluded it. Narrowing is part of the audit trail, not preprocessing."
        >
          <div className="space-y-2.5">
            <Bar
              segments={[
                ...dropped.map(([, v], i) => ({
                  value: v,
                  color: `color-mix(in srgb, var(--color-ink-faint) ${70 - i * 12}%, transparent)`,
                })),
                { value: r.narrowing.pool_after, color: "var(--color-accent)" },
              ]}
            />
            <table className="w-full">
              <tbody>
                <tr>
                  <Td>records in scope</Td>
                  <Td right mono>
                    {num(r.narrowing.pool_before)}
                  </Td>
                </tr>
                {dropped.map(([k, v]) => (
                  <tr key={k}>
                    <Td className="text-ink-faint">{constraintLabel(k)}</Td>
                    <Td right mono className="text-ink-faint">
                      −{num(v)}
                    </Td>
                  </tr>
                ))}
                <tr>
                  <Td className="text-ink">candidates</Td>
                  <Td right mono className="text-ink">
                    {num(r.narrowing.pool_after)}
                  </Td>
                </tr>
              </tbody>
            </table>

            {r.narrowing.zero_contribution_records && r.narrowing.zero_contribution_records.length > 0 && (
              <Note>
                <span className="text-ink">
                  {r.narrowing.zero_contribution_records.length} record
                  {r.narrowing.zero_contribution_records.length === 1 ? "" : "s"} net to exactly zero
                </span>{" "}
                and were set aside: a UPI payment refunded in full carries no fee to retain, so it
                moved no money and cannot be identified from the credit by any method. They are named
                on the receipt and reconciled against the declared count.
              </Note>
            )}
          </div>
        </Panel>

        {/* Stage 4: gates */}
        <Panel
          title="Can this even be answered?"
          hint="Both gates run before any search. The second one's output is the parameter the search is dispatched on."
        >
          <div className="space-y-3">
            <div className="grid grid-cols-3 gap-3">
              <Field
                label="pool"
                value={num(r.pool.n)}
                hint={`${r.pool.signed_items} negative`}
              />
              <Field label="spread σ" value={rupees(Math.round(r.pool.contribution_sigma_paise))} />
              <Field
                label="twin mass"
                value={r.amount_entropy.twin_mass.toFixed(2)}
                hint={`threshold ${r.amount_entropy.twin_mass_threshold}`}
                tone={r.amount_entropy.pass ? undefined : "var(--color-underdetermined)"}
              />
            </div>

            <div className="grid grid-cols-3 gap-3 border-t border-line-soft pt-2.5">
              <Field label="k*" value={f.k_star} hint="largest decidable free cardinality" />
              <Field
                label="collision index"
                value={idx(f.collision_index_at_k_star)}
                hint={`refuses above ${f.threshold_underdetermined}`}
                tone={f.collision_index_at_k_star > f.threshold_underdetermined ? "var(--color-underdetermined)" : undefined}
              />
              <Field
                label="analytic index"
                value={idx(f.collision_index_analytic_at_k_star)}
                hint="what the closed form says"
              />
            </div>

            <p className="text-[11.5px] leading-relaxed text-ink-faint">{f.note}</p>

            {f.collision_index_analytic_at_k_star > 0 &&
              Math.abs(
                Math.log(Math.max(f.collision_index_at_k_star, 1e-9) / f.collision_index_analytic_at_k_star),
              ) > 0.7 && (
                <Note>
                  The two estimators disagree here. The closed form assumes subset sums are locally
                  uniform near the target; real ticket amounts are lognormal and a sum of three of
                  them is not. The measured figure is the one the gate used, and both are on the
                  receipt rather than quietly reconciled.
                </Note>
              )}

            {f.implied_free_cardinality !== undefined && (
              <div className="border-t border-line-soft pt-2.5">
                <Field
                  label="declared batch"
                  value={`${f.declared_txn_count} of ${f.n}, free cardinality ${f.implied_free_cardinality}`}
                  hint={`index at that cardinality: ${idx(f.collision_index_at_implied)}`}
                />
              </div>
            )}
          </div>
        </Panel>
      </div>

      {/* Stage 5 and 6 */}
      {u && r.solver && (
        <Panel
          title="Reconstruction and proof"
          hint="Uniqueness is not a sweep bolted on after the search. It is the count the search itself produced."
          right={
            <div className="text-right text-[11px] text-ink-faint">
              <div className="tnum">{num(r.solver.entries_left + r.solver.entries_right)} entries enumerated</div>
              <div className="tnum">{(r.solver.memory_bytes / 1048576).toFixed(0)} MB</div>
            </div>
          }
        >
          <div className="grid gap-3 md:grid-cols-3">
            <Field label="witness size" value={r.witness_size} />
            <Field
              label="reconstructions found"
              value={u.matches_found}
              hint={`${u.rivals_found} rival${u.rivals_found === 1 ? "" : "s"}`}
              tone={u.rivals_found > 0 ? "var(--color-ambiguous)" : "var(--color-verified)"}
            />
            <Field label="solve side" value={r.solver.solve_side || "witness"} />
          </div>

          <div className="mt-3 rounded-md border border-line bg-raised/50 px-3 py-2">
            <div className="lbl">scope of the claim</div>
            <div className="tnum mt-1 text-[12.5px] text-ink">{u.scope}</div>
            <div className="mt-1 text-[11px] text-ink-faint">
              bounded by{" "}
              {u.scope_source === "declared_txn_count"
                ? "the settlement report's own declared transaction count"
                : "the feasibility gate, computed from the pool alone"}
            </div>
            {u.scope_note && (
              <p className="mt-2 text-[11.5px] leading-relaxed" style={{ color: "var(--color-ambiguous)" }}>
                {u.scope_note}
              </p>
            )}
          </div>

          {r.witness && r.witness.length > 0 && (
            <div className="mt-3">
              <div className="lbl mb-1">witness</div>
              <div className="flex flex-wrap gap-1">
                {r.witness.map((id) => (
                  <span
                    key={id}
                    className="tnum rounded-[3px] border border-line bg-raised px-1.5 py-0.5 text-[11px]"
                    style={
                      r.negative_members?.includes(id)
                        ? { color: "var(--color-unresolved)", borderColor: "color-mix(in srgb, var(--color-unresolved) 30%, transparent)" }
                        : undefined
                    }
                  >
                    {id}
                  </span>
                ))}
              </div>
            </div>
          )}

          {u.alternative_witnesses && u.alternative_witnesses.length > 1 && (
            <div className="mt-3 border-t border-line-soft pt-2.5">
              <div className="lbl mb-1">
                rival reconstructions, exhibited
              </div>
              <p className="mb-2 text-[11.5px] text-ink-faint">
                An ambiguous result shows its alternatives rather than asserting they exist. An analyst
                may be able to choose between these on grounds the arithmetic cannot see.
              </p>
              <div className="space-y-1.5">
                {u.alternative_witnesses.slice(0, 3).map((w, i) => (
                  <div key={i} className="tnum rounded-md border border-line px-2.5 py-1.5 text-[11px] text-ink-dim">
                    {w.join("  ")}
                  </div>
                ))}
              </div>
            </div>
          )}

          {r.solver.nearest_miss?.valid && u.matches_found === 0 && (
            <div className="mt-3 border-t border-line-soft pt-2.5">
              <div className="grid grid-cols-3 gap-3">
                <Field label="nearest achievable" value={rupees(r.solver.nearest_miss.nearest_sum_paise)} />
                <Field label="residual" value={rupees(r.solver.nearest_miss.gap_paise)} tone={tone} />
                <Field label="at cardinality" value={r.solver.nearest_miss.cardinality} />
              </div>
            </div>
          )}
        </Panel>
      )}

      {/* Accounting */}
      {r.accounting && (
        <Panel
          title="The accounting identity, re-derived"
          hint="Recomputed from the raw records without reusing any value the solver touched. If the two disagree, the solver is wrong and nothing posts."
        >
          <table className="w-full max-w-lg">
            <tbody>
              {[
                ["gross", r.accounting.gross_paise, false],
                ["merchant discount rate", -r.accounting.mdr_paise, true],
                ["GST on the fee", -r.accounting.gst_on_mdr_paise, true],
                ["refunds settled this cycle", -r.accounting.refunds_paise, true],
                ["chargebacks debited", -r.accounting.chargebacks_paise, true],
                ["adjustments", r.accounting.adjustments_paise, true],
              ].map(([label, v, dim]) => (
                <tr key={label as string}>
                  <Td className={dim ? "text-ink-faint" : ""}>{label as string}</Td>
                  <Td right mono className={dim ? "text-ink-faint" : ""}>
                    {rupees(v as number, { sign: true })}
                  </Td>
                </tr>
              ))}
              <tr>
                <Td className="text-ink">reconstructed</Td>
                <Td right mono className="text-ink">
                  {rupees(r.accounting.reconstructed_paise)}
                </Td>
              </tr>
              <tr>
                <Td className="text-ink-faint">bank credit</Td>
                <Td right mono className="text-ink-faint">
                  {rupees(r.accounting.target_paise)}
                </Td>
              </tr>
              <tr>
                <Td>
                  <span style={{ color: r.accounting.closes ? "var(--color-verified)" : "var(--color-wrong)" }}>
                    residual
                  </span>
                </Td>
                <Td right mono>
                  <span style={{ color: r.accounting.closes ? "var(--color-verified)" : "var(--color-wrong)" }}>
                    {rupees(r.accounting.residual_paise)}
                  </span>
                </Td>
              </tr>
            </tbody>
          </table>
          <p className="mt-3 text-[11.5px] text-ink-faint">
            rounding mode <span className="tnum text-ink-dim">{r.rounding.mode}</span>, tolerance{" "}
            <span className="tnum text-ink-dim">{r.rounding.tolerance_paise} paise</span> per record,
            band scaled by <span className="text-ink-dim">{r.rounding.band_basis}</span>, slack
            consumed <span className="tnum text-ink-dim">{rupees(r.rounding.slack_consumed_paise)}</span>{" "}
            of <span className="tnum text-ink-dim">{rupees(r.rounding.slack_allowed_paise)}</span>{" "}
            allowed.
          </p>
        </Panel>
      )}

      <div className="grid gap-3 lg:grid-cols-2">
        {/* Completeness */}
        <Panel
          title="Completeness guards"
          hint="The dangerous failure is a wrong posting with a proof attached. These exist for that case alone."
        >
          {probe && (
            <div className="mb-3 rounded-md border border-line px-3 py-2">
              <div className="flex items-baseline justify-between gap-3">
                <div className="text-[12px] text-ink">witness neighbourhood probe</div>
                <div
                  className="text-[11px]"
                  style={{
                    color: probe.inconclusive
                      ? "var(--color-ambiguous)"
                      : probe.stable
                        ? "var(--color-verified)"
                        : "var(--color-sensitive)",
                  }}
                >
                  {probe.inconclusive ? "inconclusive" : probe.stable ? "stable" : "rival found"}
                </div>
              </div>
              <p className="mt-1.5 text-[11.5px] leading-relaxed text-ink-faint">{probe.note}</p>
              <div className="tnum mt-2 text-[11px] text-ink-faint">
                depth {probe.max_substitution_depth} of {probe.requested_substitution_depth} requested
                · {num(probe.removal_sums_enumerated)} × {num(probe.addition_sums_enumerated)} sums ·
                pool widened to {probe.widened_pool_n} · {probe.expected_spurious_collisions.toExponential(1)}{" "}
                chance collisions expected
              </div>
              {probe.rival && (
                <div className="tnum mt-2 text-[11.5px]" style={{ color: "var(--color-sensitive)" }}>
                  {probe.rival.removed.join(", ")} → {probe.rival.added.join(", ")}, admitted by{" "}
                  {constraintLabel(probe.admitting_constraint ?? "")}
                </div>
              )}
            </div>
          )}

          <div className="space-y-2">
            {r.narrowing.completeness_checks?.map((c) => (
              <div key={c.name} className="rounded-md border border-line px-3 py-2">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="text-[12px] text-ink">{c.name.replace(/_/g, " ")}</span>
                  <span
                    className="text-[11px] tracking-wide uppercase"
                    style={{
                      color:
                        c.state === "pass"
                          ? "var(--color-verified)"
                          : c.state === "fail"
                            ? "var(--color-wrong)"
                            : "var(--color-ink-faint)",
                    }}
                  >
                    {c.state}
                  </span>
                </div>
                <p className="mt-1 text-[11.5px] leading-relaxed text-ink-faint">{c.detail}</p>
              </div>
            ))}
          </div>
        </Panel>

        {/* Fee check and agent */}
        <div className="space-y-3">
          {r.fee_check && (
            <Panel
              title="Fee check"
              hint="Whether the money is accounted for and whether the fee applied to it was right are different questions."
            >
              {r.fee_check.circular ? (
                <Note tone="var(--color-underdetermined)">{r.fee_check.claim}</Note>
              ) : (
                <>
                  <div className="grid grid-cols-3 gap-3">
                    <Field label="expected" value={rupees(r.fee_check.expected_mdr_paise)} />
                    <Field label="observed" value={rupees(r.fee_check.observed_mdr_paise)} />
                    <Field
                      label="delta"
                      value={`${r.fee_check.delta_bps} bps`}
                      hint={`band ${r.fee_check.band_bps} bps`}
                      tone={r.fee_check.within_band ? "var(--color-verified)" : "var(--color-ambiguous)"}
                    />
                  </div>
                  <p className="mt-3 text-[11.5px] leading-relaxed text-ink-faint">{r.fee_check.claim}</p>
                </>
              )}
            </Panel>
          )}

          {r.agent.invoked && (
            <Panel
              title="Resolution agent"
              hint="The model proposes. The unmodified verifier disposes. It is never asked whether it was right."
              right={
                <span className="tnum text-[11px] text-ink-faint">
                  {r.agent.provider} · {r.agent.iterations} iteration
                  {r.agent.iterations === 1 ? "" : "s"}
                </span>
              }
            >
              <div className="space-y-2">
                {r.agent.hypotheses?.map((h, i) => {
                  const accepted = h.outcome?.startsWith("accepted");
                  return (
                    <div
                      key={i}
                      className="rounded border px-3 py-2"
                      style={{
                        borderColor: accepted
                          ? "color-mix(in srgb, var(--color-verified) 35%, transparent)"
                          : "var(--color-line)",
                        background: accepted
                          ? "color-mix(in srgb, var(--color-verified) 7%, transparent)"
                          : undefined,
                      }}
                    >
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="tnum text-[12px] text-ink">
                          {h.kind.replace(/_/g, " ").toLowerCase()}
                        </span>
                        <span className="tnum text-[12px] text-ink-dim">{rupees(h.amount_paise)}</span>
                      </div>
                      {h.rationale && (
                        <p className="mt-1 text-[11.5px] leading-relaxed text-ink-faint">{h.rationale}</p>
                      )}
                      {h.source_ref ? (
                        <p className="tnum mt-1.5 text-[11px]" style={{ color: "var(--color-verified)" }}>
                          cites {h.source_ref} — {h.evidence}
                        </p>
                      ) : (
                        <p className="mt-1.5 text-[11px] text-ink-faint">
                          uncited, so it can never post whatever the arithmetic says
                        </p>
                      )}
                      <p className="mt-1 text-[11px] text-ink-faint">{h.outcome}</p>
                    </div>
                  );
                })}
              </div>
              {r.agent.note && (
                <p className="mt-3 text-[11.5px] leading-relaxed text-ink-faint">{r.agent.note}</p>
              )}
            </Panel>
          )}
        </div>
      </div>

      {/* Remediation */}
      {r.remediation && r.remediation.length > 0 && (
        <Panel
          title="What would change this"
          hint="Not advice. Where possible, the collision index the named change is estimated to produce."
        >
          <div className="space-y-2">
            {r.remediation.map((rm, i) => (
              <div key={i} className="rounded-md border border-line px-3 py-2">
                <div className="text-[12px] text-ink">{rm.action}</div>
                <div className="mt-0.5 text-[11.5px] text-ink-faint">{rm.effect}</div>
                {rm.projected_collision_index !== undefined && (
                  <div className="tnum mt-1 text-[11px]" style={{ color: "var(--color-accent)" }}>
                    projected index {idx(rm.projected_collision_index)}
                    {rm.projected_pool_n !== undefined && ` at a pool of ${rm.projected_pool_n}`}
                  </div>
                )}
              </div>
            ))}
          </div>
        </Panel>
      )}

      {/* Timings */}
      <Panel title="Where the time went">
        <table className="w-full max-w-md">
          <thead>
            <tr>
              <Th>stage</Th>
              <Th right>ms</Th>
            </tr>
          </thead>
          <tbody>
            {Object.entries(r.timing_ms).map(([k, v]) => (
              <tr key={k}>
                <Td className="text-ink-faint">{k.replace(/_/g, " ")}</Td>
                <Td right mono>
                  {v}
                </Td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-[11.5px] text-ink-faint">
          There is no separate line for the uniqueness proof, because the reconstruction step
          produced the count. Policy <span className="tnum text-ink-dim">{r.policy_version}</span>,
          replay seed <span className="tnum text-ink-dim">{r.replay_seed}</span>.
        </p>
      </Panel>
    </div>
  );
}
