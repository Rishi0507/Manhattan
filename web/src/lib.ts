import type { Status } from "./types";

/**
 * Format an integer paise amount in Indian digit grouping.
 *
 * Amounts arrive as integers and stay integers. There is no path in this
 * interface where an amount becomes a float, because the reason the backend
 * uses integer paise is that floating point makes exact verification
 * impossible, and displaying it through a lossy conversion would quietly
 * undo that.
 */
export function rupees(paise: number, opts: { sign?: boolean; symbol?: boolean } = {}): string {
  const { sign = false, symbol = true } = opts;
  const neg = paise < 0;
  const v = Math.abs(paise);
  const whole = Math.floor(v / 100);
  const frac = v % 100;

  const s = String(whole);
  let head = s.length > 3 ? s.slice(0, -3) : "";
  const tail = s.length > 3 ? s.slice(-3) : s;
  const parts: string[] = [];
  while (head.length > 2) {
    parts.unshift(head.slice(-2));
    head = head.slice(0, -2);
  }
  if (head) parts.unshift(head);
  const grouped = parts.length ? parts.join(",") + "," + tail : tail;

  const prefix = neg ? "-" : sign ? "+" : "";
  return `${prefix}${symbol ? "₹" : ""}${grouped}.${String(frac).padStart(2, "0")}`;
}

/** Compact rupee rendering for dense tables: 4.86L, 1.2Cr. */
export function rupeesShort(paise: number): string {
  const v = Math.abs(paise) / 100;
  const neg = paise < 0 ? "-" : "";
  if (v >= 1e7) return `${neg}₹${(v / 1e7).toFixed(2)}Cr`;
  if (v >= 1e5) return `${neg}₹${(v / 1e5).toFixed(2)}L`;
  if (v >= 1e3) return `${neg}₹${(v / 1e3).toFixed(1)}k`;
  return `${neg}₹${v.toFixed(0)}`;
}

/** Render a collision index, which spans twenty orders of magnitude. */
export function idx(v: number | undefined): string {
  if (v === undefined || v === null) return "—";
  if (v >= 1e17) return "astronomical";
  if (v === 0) return "0";
  if (v < 0.001) return v.toExponential(1);
  if (v < 10) return v.toFixed(3).replace(/0+$/, "").replace(/\.$/, "");
  if (v < 1e6) return Math.round(v).toLocaleString("en-IN");
  return v.toExponential(1);
}

export function pct(v: number, digits = 0): string {
  return `${(v * 100).toFixed(digits)}%`;
}

export function num(v: number): string {
  return v.toLocaleString("en-IN");
}

/** Colour token per status. */
export function statusColor(s: Status): string {
  switch (s) {
    case "VERIFIED":
      return "var(--color-verified)";
    case "AMBIGUOUS":
      return "var(--color-ambiguous)";
    case "UNDERDETERMINED":
      return "var(--color-underdetermined)";
    case "NARROWING_SENSITIVE":
      return "var(--color-sensitive)";
    case "UNRESOLVED":
      return "var(--color-unresolved)";
  }
}

/**
 * A single glyph per status, so a column of them scans without reading.
 *
 * Carrying status by glyph plus a muted tint, rather than by five saturated
 * colours, keeps the table legible: the data should be the loudest thing on
 * the screen.
 */
export function statusGlyph(s: Status): string {
  switch (s) {
    case "VERIFIED":
      return "✓";
    case "AMBIGUOUS":
      return "≈";
    case "UNDERDETERMINED":
      return "∅";
    case "NARROWING_SENSITIVE":
      return "⚠";
    case "UNRESOLVED":
      return "?";
  }
}

/**
 * One line on what each status means, in the words a finance lead would use.
 *
 * These are shown in the interface rather than kept in documentation because
 * the five-status model is the product, and a viewer who reads
 * "UNDERDETERMINED" as a synonym for "failed" has missed the entire argument.
 */
export function statusMeaning(s: Status): string {
  switch (s) {
    case "VERIFIED":
      return "Exactly one reconstruction exists in the region searched, the count was exhaustive, and the accounting identity closes. This is the only status that posts.";
    case "AMBIGUOUS":
      return "Two or more reconstructions fit. Both are exhibited, because an analyst may be able to choose between them on grounds the arithmetic cannot see.";
    case "UNDERDETERMINED":
      return "The combinatorics guarantee a large population of reconstructions. Showing one would misrepresent it as an answer. The remedy is more data, not more search.";
    case "NARROWING_SENSITIVE":
      return "A reconstruction was found, but widening the pool admits a rival. The answer came from a filtering decision rather than from the arithmetic.";
    case "UNRESOLVED":
      return "Nothing reconstructs this credit within the declared tolerance. The exact residual is carried, and the agent gets one bounded, cited attempt to explain it.";
  }
}

export function flagMeaning(f: string): string {
  const m: Record<string, string> = {
    SIGNED_ITEMS_PRESENT:
      "The batch contains negative contributions: a chargeback debit, or a payment refunded in full whose fee was retained.",
    FEE_ANOMALY:
      "The money is accounted for, but the effective fee rate applied to it sits outside the configured band.",
    FEE_CHECK_CIRCULAR:
      "In this data mode the observed fee derives from the same policy that built the contributions, so the check cannot fail. No anomaly claim is made.",
    ROUNDING_APPLIED:
      "The rounding convention was unknown, so a tolerance scaled by the witness cardinality was allowed. The slack consumed is reported.",
    RESOLVED_BY_HYPOTHESIS:
      "An agent hypothesis, backed by a real cited record, closed the gap. The verifier re-ran unmodified over the amended pool.",
    COMPLEMENT_SOLVED:
      "The batch was almost the whole pool, so the answer was recovered by solving for the excluded set in the same enumeration.",
    TWIN_SWAP:
      "Two records carry an identical contribution, so an alternative reconstruction is constructible. The ambiguity is structural, not incidental.",
    LATTICE_CORRECTED:
      "Every contribution shares a common divisor, so achievable sums are that many times denser than a continuous model assumes. The index was corrected.",
    AMOUNT_ENTROPY_INSUFFICIENT:
      "Too much of the pool sits in classes of identical amounts. These settlements are not reconstructable from amounts by any method.",
    RESOURCE_CEILING:
      "The region is decidable in principle, but enumerating it exceeds the configured memory ceiling.",
  };
  return m[f] ?? f;
}

export function shortFlag(f: string): string {
  const m: Record<string, string> = {
    SIGNED_ITEMS_PRESENT: "signed",
    FEE_ANOMALY: "fee anomaly",
    FEE_CHECK_CIRCULAR: "fee circular",
    ROUNDING_APPLIED: "rounding",
    RESOLVED_BY_HYPOTHESIS: "agent cited",
    COMPLEMENT_SOLVED: "complement",
    TWIN_SWAP: "twin",
    LATTICE_CORRECTED: "lattice",
    AMOUNT_ENTROPY_INSUFFICIENT: "no entropy",
    RESOURCE_CEILING: "ceiling",
  };
  return m[f] ?? f.toLowerCase().replace(/_/g, " ");
}

export function constraintLabel(c: string): string {
  const m: Record<string, string> = {
    mid_mismatch: "different merchant",
    currency_mismatch: "different currency",
    outside_settlement_window: "outside the value-date window",
    already_reconciled: "posted in a prior cycle",
    payment_method_mismatch: "different payment method",
    settlement_reference_mismatch: "different settlement reference",
    zero_net_contribution: "nets to exactly zero",
  };
  return m[c] ?? c.replace(/_/g, " ");
}

/** A tiny fetch wrapper that surfaces server errors as thrown messages. */
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export function cls(...xs: (string | false | null | undefined)[]): string {
  return xs.filter(Boolean).join(" ");
}
