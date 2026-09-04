/**
 * The hero figure: a settlement being reconstructed.
 *
 * The empty half of the hero was doing nothing, and a decorative graphic there
 * would have been worse than nothing. This draws the actual mechanism instead:
 * a column of payment amounts, each with its fee deducted, summing to one bank
 * credit, and then a second candidate subset being tested against the same
 * total and rejected.
 *
 * That second half is the part most reconciliation graphics leave out, and it
 * is the whole argument. Finding one subset that sums correctly is easy.
 * Proving no other subset does is the property that makes a posting safe.
 *
 * It is inline SVG with CSS animation, no library and no canvas, so it costs
 * nothing to load and scales cleanly. Motion is suppressed under
 * prefers-reduced-motion, where it renders as a static diagram that still reads
 * correctly.
 */
export function ReconstructionFigure() {
  // Amounts in paise, chosen so the arithmetic on screen is real: the four
  // credited rows net to the settlement exactly.
  const rows = [
    { gross: 48250, fee: 1139 },
    { gross: 91400, fee: 2158 },
    { gross: 30975, fee: 731 },
    { gross: 77300, fee: 1825 },
  ];
  const net = rows.map((r) => r.gross - r.fee);
  const total = net.reduce((a, b) => a + b, 0);
  const inr = (p: number) =>
    "\u20b9" + (p / 100).toLocaleString("en-IN", { minimumFractionDigits: 2 });

  return (
    <figure className="select-none" aria-label="A settlement reconstructed from four payments, with a rival subset rejected.">
      <div className="rounded-lg border border-line bg-ground/70 p-5 sm:p-6">
        <p className="lbl">the credit</p>
        <p className="tnum mt-1 text-[26px] leading-none font-medium text-ink sm:text-[30px]">
          {inr(total)}
        </p>

        <div className="mt-5 space-y-[7px]">
          {rows.map((r, i) => (
            <div
              key={i}
              className="mh-row flex items-center gap-3"
              style={{ animationDelay: `${i * 0.16}s` }}
            >
              <span
                className="block h-[9px] rounded-[2px] bg-accent/85"
                style={{ width: `${(net[i] / 95000) * 100}%` }}
              />
              <span className="tnum text-[11.5px] whitespace-nowrap text-ink-faint">
                {inr(r.gross)} <span className="text-ink-faint/70">less fee {inr(r.fee)}</span>
              </span>
            </div>
          ))}
        </div>

        <div className="mh-sum mt-4 border-t border-line-strong pt-3">
          <div className="flex items-baseline justify-between gap-4">
            <span className="text-[12.5px] text-ink-dim">sums exactly</span>
            <span className="tnum text-[13px] font-medium text-ink">{inr(total)}</span>
          </div>
        </div>

        <div className="mh-rival mt-4 rounded-md border border-dashed border-line-strong px-3 py-2.5">
          <div className="flex items-baseline justify-between gap-4">
            <span className="text-[12.5px] text-ink-dim">rival subset tested</span>
            <span className="tnum text-[13px] text-ink-faint">off by {inr(4150)}</span>
          </div>
          <p className="mt-1.5 text-[11.5px] leading-snug text-ink-faint">
            No other subset reaches the total. That is the proof, and it is what
            separates a posting from a guess.
          </p>
        </div>
      </div>

      <style>{`
        @keyframes mhSlide {
          from { opacity: 0; transform: translateX(-10px); }
          to   { opacity: 1; transform: none; }
        }
        @keyframes mhRise {
          from { opacity: 0; transform: translateY(6px); }
          to   { opacity: 1; transform: none; }
        }
        .mh-row   { animation: mhSlide .55s cubic-bezier(.22,.61,.36,1) both; }
        .mh-sum   { animation: mhRise .5s cubic-bezier(.22,.61,.36,1) .78s both; }
        .mh-rival { animation: mhRise .5s cubic-bezier(.22,.61,.36,1) 1.05s both; }
        @media (prefers-reduced-motion: reduce) {
          .mh-row, .mh-sum, .mh-rival { animation: none; opacity: 1; transform: none; }
        }
      `}</style>
    </figure>
  );
}
