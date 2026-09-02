import type { ReactNode } from "react";
import type { Status } from "./types";
import { cls, shortFlag, statusColor } from "./lib";

/**
 * The whole component vocabulary for this interface, which is deliberately
 * about eight things.
 *
 * An interface for reading financial decisions wants a small number of
 * elements used consistently, not a large number used once each. Everything
 * here is a thin wrapper over a div with a border.
 */

export function StatusPill({ status, size = "md" }: { status: Status; size?: "sm" | "md" }) {
  const c = statusColor(status);
  return (
    <span
      className={cls(
        "inline-flex items-center gap-1.5 rounded-[3px] border font-medium whitespace-nowrap",
        size === "sm" ? "px-1.5 py-px text-[10px] tracking-wide" : "px-2 py-0.5 text-[11px] tracking-wide",
      )}
      style={{ color: c, borderColor: `color-mix(in srgb, ${c} 32%, transparent)`, background: `color-mix(in srgb, ${c} 9%, transparent)` }}
    >
      <span className="size-1 rounded-full" style={{ background: c }} />
      {status.replace(/_/g, " ")}
    </span>
  );
}

export function Flag({ name, title }: { name: string; title?: string }) {
  return (
    <span
      title={title}
      className="inline-flex items-center rounded-[3px] border border-line bg-raised px-1.5 py-px text-[10px] tracking-wide text-ink-dim"
    >
      {shortFlag(name)}
    </span>
  );
}

export function Panel({
  title,
  subtitle,
  right,
  children,
  className,
}: {
  title?: ReactNode;
  subtitle?: ReactNode;
  right?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cls("rounded border border-line bg-surface", className)}>
      {(title || right) && (
        <header className="flex items-start justify-between gap-4 border-b border-line-soft px-4 py-3">
          <div className="min-w-0">
            {title && <h2 className="text-[13px] font-medium text-ink">{title}</h2>}
            {subtitle && <p className="mt-0.5 text-[12px] leading-snug text-ink-faint">{subtitle}</p>}
          </div>
          {right && <div className="shrink-0">{right}</div>}
        </header>
      )}
      <div className="p-4">{children}</div>
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
      <div className="text-[10.5px] tracking-wide text-ink-faint uppercase">{label}</div>
      <div
        className={cls("mt-0.5 truncate text-[13px]", mono && "tnum")}
        style={tone ? { color: tone } : undefined}
        title={typeof value === "string" ? value : undefined}
      >
        {value}
      </div>
      {hint && <div className="mt-0.5 text-[11px] leading-snug text-ink-faint">{hint}</div>}
    </div>
  );
}

export function Stat({
  label,
  value,
  sub,
  tone,
  emphasis,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  tone?: string;
  emphasis?: boolean;
}) {
  return (
    <div className="rounded border border-line bg-surface px-3.5 py-3">
      <div className="text-[10.5px] tracking-wide text-ink-faint uppercase">{label}</div>
      <div
        className={cls("tnum mt-1 leading-none", emphasis ? "text-[26px]" : "text-[20px]")}
        style={tone ? { color: tone } : undefined}
      >
        {value}
      </div>
      {sub && <div className="mt-1.5 text-[11px] leading-snug text-ink-faint">{sub}</div>}
    </div>
  );
}

/**
 * A horizontal proportion bar. Used for the narrowing waterfall and the
 * status mix, both of which are about how a whole divides.
 */
export function Bar({
  segments,
  height = 6,
}: {
  segments: { value: number; color: string; label?: string }[];
  height?: number;
}) {
  const total = segments.reduce((a, s) => a + s.value, 0) || 1;
  return (
    <div className="flex w-full overflow-hidden rounded-[2px]" style={{ height }}>
      {segments.map((s, i) => (
        <div
          key={i}
          title={s.label}
          style={{ width: `${(s.value / total) * 100}%`, background: s.color }}
        />
      ))}
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
    <div className="flex gap-0.5 overflow-x-auto" role="tablist">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={active === t.id}
          onClick={() => onChange(t.id)}
          className={cls(
            "relative whitespace-nowrap rounded-t border-b-2 px-3.5 py-2 text-[12.5px] transition-colors",
            active === t.id
              ? "border-accent text-ink"
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
      className="rounded border-l-2 bg-raised/60 py-2 pr-3 pl-3 text-[12px] leading-relaxed text-ink-dim"
      style={{ borderColor: tone ?? "var(--color-line)" }}
    >
      {children}
    </p>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="rounded border border-dashed border-line px-4 py-10 text-center text-[12.5px] text-ink-faint">
      {children}
    </div>
  );
}

export function Th({ children, right }: { children: ReactNode; right?: boolean }) {
  return (
    <th
      className={cls(
        "border-b border-line px-2.5 py-2 text-[10.5px] font-medium tracking-wide text-ink-faint uppercase",
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
  className,
}: {
  children: ReactNode;
  right?: boolean;
  mono?: boolean;
  className?: string;
}) {
  return (
    <td
      className={cls(
        "border-b border-line-soft px-2.5 py-1.5 text-[12.5px]",
        right && "text-right",
        mono && "tnum",
        className,
      )}
    >
      {children}
    </td>
  );
}
