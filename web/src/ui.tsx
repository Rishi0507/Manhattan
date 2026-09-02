import type { ReactNode } from "react";
import type { Status } from "./types";
import { cls, shortFlag, statusColor, statusGlyph } from "./lib";

/**
 * The whole component vocabulary, which is deliberately about nine things.
 *
 * An interface for reading financial decisions wants a small number of
 * elements used consistently, not a large number used once each. Everything
 * here is a thin wrapper over a div with a border, sized for density: 26px
 * table rows, 12px body, 10px labels.
 */

export function StatusPill({ status, size = "md" }: { status: Status; size?: "sm" | "md" }) {
  const c = statusColor(status);
  return (
    <span
      className={cls(
        "inline-flex items-center gap-1 rounded-[3px] font-medium whitespace-nowrap",
        size === "sm" ? "px-1 py-px text-[10px]" : "px-1.5 py-0.5 text-[11px]",
      )}
      style={{ color: c, background: `color-mix(in srgb, ${c} 9%, transparent)` }}
      title={status}
    >
      <span aria-hidden className="font-mono">
        {statusGlyph(status)}
      </span>
      {status === "NARROWING_SENSITIVE" ? "SENSITIVE" : status}
    </span>
  );
}

export function Flag({ name, title }: { name: string; title?: string }) {
  return (
    <span
      title={title ?? name}
      className="inline-flex items-center rounded-[3px] bg-sunken px-1.5 py-px text-[10px] whitespace-nowrap text-ink-dim"
    >
      {shortFlag(name)}
    </span>
  );
}

export function Panel({
  title,
  hint,
  right,
  children,
  flush,
  className,
}: {
  title?: ReactNode;
  hint?: ReactNode;
  right?: ReactNode;
  children: ReactNode;
  flush?: boolean;
  className?: string;
}) {
  return (
    <section className={cls("rounded-md border border-line bg-surface", className)}>
      {(title || right) && (
        <header className="flex items-center justify-between gap-4 border-b border-line-soft px-3.5 py-2">
          <div className="flex min-w-0 items-baseline gap-2.5">
            {title && <h2 className="text-[12.5px] font-semibold text-ink">{title}</h2>}
            {hint && <p className="truncate text-[11.5px] text-ink-faint">{hint}</p>}
          </div>
          {right && <div className="shrink-0">{right}</div>}
        </header>
      )}
      <div className={flush ? "" : "p-3.5"}>{children}</div>
    </section>
  );
}

/** A label above a value, which is most of this interface. */
export function Field({
  label,
  value,
  hint,
  tone,
  mono = true,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <div className="lbl">{label}</div>
      <div
        className={cls("mt-0.5 truncate text-[12.5px]", mono && "tnum")}
        style={tone ? { color: tone } : undefined}
        title={typeof value === "string" ? value : undefined}
      >
        {value}
      </div>
      {hint && <div className="mt-px text-[11px] leading-snug text-ink-faint">{hint}</div>}
    </div>
  );
}

/**
 * The summary bar. One row, everything that matters, no scrolling.
 *
 * This exists because the first complaint about any dashboard is that you
 * cannot take it in at a glance. Six figures on one line, largest first.
 */
export function SummaryBar({ items }: { items: { label: string; value: ReactNode; sub?: ReactNode; tone?: string }[] }) {
  return (
    <div className="grid grid-cols-2 divide-line-soft rounded-md border border-line bg-surface sm:grid-cols-3 lg:grid-cols-6 lg:divide-x">
      {items.map((it, i) => (
        <div key={i} className="border-line-soft px-3.5 py-2.5 not-last:max-lg:border-b">
          <div className="lbl">{it.label}</div>
          <div className="tnum mt-0.5 text-[19px] leading-tight" style={it.tone ? { color: it.tone } : undefined}>
            {it.value}
          </div>
          {it.sub && <div className="mt-px truncate text-[11px] text-ink-faint">{it.sub}</div>}
        </div>
      ))}
    </div>
  );
}

/** A horizontal proportion bar, for the narrowing waterfall and status mix. */
export function Bar({
  segments,
  height = 5,
}: {
  segments: { value: number; color: string; label?: string }[];
  height?: number;
}) {
  const total = segments.reduce((a, s) => a + s.value, 0) || 1;
  return (
    <div className="flex w-full overflow-hidden rounded-[2px] bg-sunken" style={{ height }}>
      {segments.map((s, i) =>
        s.value <= 0 ? null : (
          <div key={i} title={s.label} style={{ width: `${(s.value / total) * 100}%`, background: s.color }} />
        ),
      )}
    </div>
  );
}

export function Tabs<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: { id: T; label: string; badge?: ReactNode }[];
  active: T;
  onChange: (id: T) => void;
}) {
  return (
    <div className="-mb-px flex gap-0.5 overflow-x-auto" role="tablist">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={active === t.id}
          onClick={() => onChange(t.id)}
          className={cls(
            "relative -mb-px whitespace-nowrap border-b-2 px-3 py-2 text-[12.5px] transition-colors",
            active === t.id
              ? "border-accent font-medium text-ink"
              : "border-transparent text-ink-faint hover:text-ink-dim",
          )}
        >
          {t.label}
          {t.badge != null && <span className="tnum ml-1.5 text-[11px] text-ink-faint">{t.badge}</span>}
        </button>
      ))}
    </div>
  );
}

/** A one-line callout, used where the interface needs to say something. */
export function Note({ children, tone }: { children: ReactNode; tone?: string }) {
  return (
    <p
      className="rounded-[3px] border-l-2 bg-raised px-3 py-2 text-[11.5px] leading-relaxed text-ink-dim"
      style={{ borderColor: tone ?? "var(--color-line-strong)" }}
    >
      {children}
    </p>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md border border-dashed border-line px-4 py-12 text-center text-[12px] text-ink-faint">
      {children}
    </div>
  );
}

export function Th({ children, right, w }: { children: ReactNode; right?: boolean; w?: string }) {
  return (
    <th
      style={w ? { width: w } : undefined}
      className={cls(
        "lbl sticky top-0 z-10 border-b border-line bg-surface px-2.5 py-1.5",
        right ? "text-right" : "text-left",
      )}
    >
      {children}
    </th>
  );
}

export function Td({
  children,
  right,
  mono,
  dim,
  className,
}: {
  children: ReactNode;
  right?: boolean;
  mono?: boolean;
  dim?: boolean;
  className?: string;
}) {
  return (
    <td
      className={cls(
        "border-b border-line-soft px-2.5 py-1 text-[12px]",
        right && "text-right",
        mono && "tnum",
        dim && "text-ink-faint",
        className,
      )}
    >
      {children}
    </td>
  );
}

/** A dense definition row, for the accounting identity and similar. */
export function Row({
  label,
  value,
  dim,
  tone,
  strong,
}: {
  label: ReactNode;
  value: ReactNode;
  dim?: boolean;
  tone?: string;
  strong?: boolean;
}) {
  return (
    <div
      className={cls(
        "flex items-baseline justify-between gap-4 border-b border-line-soft py-1 last:border-0",
        strong && "border-line-strong",
      )}
    >
      <span className={cls("text-[12px]", dim ? "text-ink-faint" : "text-ink-dim")}>{label}</span>
      <span className="tnum text-[12px]" style={tone ? { color: tone } : undefined}>
        {value}
      </span>
    </div>
  );
}
