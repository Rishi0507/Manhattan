import type { Summary } from "./types";
import { Empty, Note, Panel } from "./ui";
import { num } from "./lib";

/**
 * The period close.
 *
 * This is the first tab because it is the only view in the dashboard that
 * answers the question an operations lead actually arrives with. Every other
 * tab shows what happened to settlements; this one shows what to do on Monday.
 *
 * The grading block is not decoration. The benchmark injects specific
 * operational misconfigurations and records what they are, and none of that
 * reaches the model: it reads status mixes, pool sizes, twin masses and held
 * values, and has to infer the cause the way a person would. Showing the score
 * beside the findings is what separates this from a generated summary nobody
 * can check.
 */
export function Close({ summary }: { summary: Summary | null }) {
  const pc = summary?.period_close;
  if (!pc) {
    return (
      <Empty>
        No close in this run. It is written once per batch, after every settlement has been
        decided. Run <code className="tnum">./run.sh bench</code>.
      </Empty>
    );
  }

  const injected = pc.conditions_injected ?? [];
  const found = pc.conditions_found ?? [];
  const missed = pc.conditions_missed ?? [];
  const spurious = pc.findings_not_injected ?? [];

  return (
    <div className="space-y-3">
      <Panel
        title="The close"
        hint="written once per period, over aggregates, by the model"
      >
        <p className="max-w-[78ch] text-[14.5px] leading-relaxed text-ink">{pc.narrative}</p>
        <p className="mt-3 text-[12.5px] leading-relaxed text-ink-faint">
          The close cannot act. It posts nothing, narrows nothing, amends no input and alters no
          receipt, which is exactly why it is the one model output here not bounded by a closed
          action vocabulary. A person reads it and then decides.
        </p>
      </Panel>

      {injected.length > 0 && (
        <Panel title="Graded" hint="the model was never told what this run got wrong">
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="conditions injected" value={String(injected.length)} />
            <Field
              label="identified, right merchant"
              value={String(found.length)}
              tone="var(--color-verified)"
            />
            <Field
              label="recall"
              value={`${Math.round((pc.condition_recall ?? 0) * 100)}%`}
              tone={missed.length === 0 ? "var(--color-verified)" : "var(--color-ambiguous)"}
            />
          </div>

          <div className="mt-3 space-y-1.5 border-t border-line-soft pt-2.5">
            {injected.map((c) => {
              const scope = c.split(":")[0];
              const hit = found.some((f) => f.startsWith(scope + ":"));
              return (
                <div key={c} className="flex items-baseline gap-3">
                  <span
                    className="tnum w-[68px] shrink-0 text-[11.5px]"
                    style={{ color: hit ? "var(--color-verified)" : "var(--color-wrong)" }}
                  >
                    {hit ? "found" : "MISSED"}
                  </span>
                  <span className="text-[12.5px] leading-snug text-ink-dim">{c}</span>
                </div>
              );
            })}
          </div>

          {spurious.length > 0 && (
            <Note>
              Findings matching no injected condition: {spurious.join(", ")}. Listed rather than
              counted against recall, because at least some are true. The flat-price merchants
              genuinely cannot be reconstructed from amounts, and saying so is correct even though
              nobody injected it.
            </Note>
          )}
        </Panel>
      )}

      <Panel flush title="Root causes" hint="ranked by held value, each citing its evidence">
        <div className="overflow-x-auto">
          <table className="wide w-full border-separate border-spacing-0">
            <thead>
              <tr>
                <Th w="150px">scope</Th>
                <Th w="220px">cause</Th>
                <Th right w="110px">held INR</Th>
                <Th>evidence it cited</Th>
                <Th w="260px">recommended action</Th>
              </tr>
            </thead>
            <tbody>
              {(pc.root_causes ?? []).map((rc, i) => (
                <tr key={i}>
                  <Td mono dim>
                    {rc.scope.replace(/_/g, " ")}
                  </Td>
                  <Td mono>{rc.cause_class.replace(/_/g, " ").toLowerCase()}</Td>
                  <Td right mono>
                    {num(rc.value_held_inr)}
                  </Td>
                  <Td className="text-ink-faint">{rc.evidence}</Td>
                  <Td className="text-ink-dim">{rc.recommended_action}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <div className="grid gap-3 lg:grid-cols-2">
        {(pc.escalations ?? []).length > 0 && (
          <Panel title="Needs a human" hint="what the close will not decide">
            <ul className="space-y-2">
              {(pc.escalations ?? []).map((e, i) => (
                <li key={i} className="text-[13px] leading-relaxed text-ink-dim">
                  {e}
                </li>
              ))}
            </ul>
          </Panel>
        )}
        {pc.what_i_cannot_tell && (
          <Panel title="What it says it cannot tell" hint="the most useful paragraph in any report">
            <p className="text-[13px] leading-relaxed text-ink-dim">{pc.what_i_cannot_tell}</p>
          </Panel>
        )}
      </div>
    </div>
  );
}

/** A small labelled figure, local so this file stands alone. */
function Field({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <div className="lbl">{label}</div>
      <div className="tnum mt-1 text-[22px] leading-none" style={{ color: tone ?? undefined }}>
        {value}
      </div>
    </div>
  );
}

function Th({
  children,
  right,
  w,
}: {
  children?: React.ReactNode;
  right?: boolean;
  w?: string;
}) {
  return (
    <th
      style={w ? { width: w } : undefined}
      className={`lbl border-b border-line bg-raised/60 px-3 py-2 ${right ? "text-right" : "text-left"}`}
    >
      {children}
    </th>
  );
}

function Td({
  children,
  right,
  mono,
  dim,
  className = "",
}: {
  children?: React.ReactNode;
  right?: boolean;
  mono?: boolean;
  dim?: boolean;
  className?: string;
}) {
  return (
    <td
      className={[
        "border-b border-line-soft px-3 py-2 align-top text-[12.5px] leading-snug",
        right ? "text-right" : "",
        mono ? "tnum" : "",
        dim ? "text-ink-faint" : "",
        className,
      ].join(" ")}
    >
      {children}
    </td>
  );
}
