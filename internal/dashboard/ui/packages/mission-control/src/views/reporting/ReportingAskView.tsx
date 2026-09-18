import { useMemo, useState } from "react";
import { reportingRoute, type Route } from "../../lib/routes";
import { StatusBadge, tileName } from "../../components/shared";
import { Link } from "../../components/Link";
import type { KPIValue, SummaryTile } from "../../lib/types";

interface Props {
  route: Route;
  tiles: SummaryTile[];
}

interface Answer {
  text: string;
  cites: SummaryTile[];
}

function sev(t: SummaryTile): string {
  return (t.status || "unknown").toLowerCase();
}

function inCategory(t: SummaryTile, cat: string): boolean {
  return (t.categories || []).includes(cat) || tileName(t) === cat;
}

function kpi(t: SummaryTile, name: string): KPIValue | undefined {
  return (t.kpis || []).find((k) => k.name === name);
}

function sumKpi(tiles: SummaryTile[], cat: string, name: string): number {
  let n = 0;
  for (const t of tiles.filter((x) => inCategory(x, cat))) n += kpi(t, name)?.value || 0;
  return n;
}

const QUESTIONS: Array<{ q: string; keys: string[]; answer: (tiles: SummaryTile[]) => Answer }> = [
  {
    q: "What needs attention?",
    keys: ["attention", "wrong", "broken", "bad", "status", "problem"],
    answer: (tiles) => {
      const bad = tiles.filter((t) => sev(t) === "critical" || sev(t) === "warn");
      return {
        text: bad.length
          ? bad.length +
            (bad.length === 1 ? " report is" : " reports are") +
            " not healthy: " +
            bad.map(tileName).join(", ") +
            "."
          : "Nothing is critical or warning at this scope.",
        cites: bad,
      };
    },
  },
  {
    q: "What needs a human?",
    keys: ["human", "decision", "approve", "escalat", "blocked", "waiting"],
    answer: (tiles) => {
      const d = tiles.filter((t) => inCategory(t, "decisions"));
      const waiting = sumKpi(tiles, "decisions", "awaiting_human");
      const open = sumKpi(tiles, "decisions", "open_decisions");
      return {
        text: d.length
          ? waiting +
            (waiting === 1 ? " decision is" : " decisions are") +
            " waiting on a person, out of " +
            open +
            " open."
          : "No decisions reports at this scope, so nothing can be said about what is blocked on a human.",
        cites: d,
      };
    },
  },
  {
    q: "Where are the bottlenecks?",
    keys: ["bottleneck", "slow", "stuck", "queue", "block"],
    answer: (tiles) => {
      const d = tiles.filter((t) => inCategory(t, "decisions"));
      const p = tiles.filter((t) => inCategory(t, "performance"));
      const waiting = sumKpi(tiles, "decisions", "awaiting_human");
      const cites = [...d, ...p];
      return {
        text:
          (cites.length
            ? "A bottleneck is not a report of its own — it is read across decisions and performance. " +
              waiting +
              " item(s) are waiting on a human" +
              (p.length ? ", and " + p.length + " performance report(s) cover throughput and latency here." : ".")
            : "Neither decisions nor performance reports exist at this scope, so a bottleneck cannot be located.") +
          " See Docs → Categories for why this is a question rather than a category.",
        cites,
      };
    },
  },
  {
    q: "What did we ship?",
    keys: ["ship", "deliver", "deploy", "release"],
    answer: (tiles) => {
      const d = tiles.filter((t) => inCategory(t, "delivery"));
      const deploys = sumKpi(tiles, "delivery", "deploys");
      const failed = sumKpi(tiles, "delivery", "failed");
      return {
        text: d.length
          ? deploys + " deploy(s) recorded, " + failed + " of them failed."
          : "No delivery reports at this scope.",
        cites: d,
      };
    },
  },
  {
    q: "What is this costing?",
    keys: ["cost", "spend", "token", "budget", "money"],
    answer: (tiles) => {
      const c = tiles.filter((t) => inCategory(t, "cost"));
      const spend = sumKpi(tiles, "cost", "spend_usd");
      const tokens = sumKpi(tiles, "cost", "tokens");
      return {
        text: c.length
          ? spend.toLocaleString() + " USD across " + tokens.toLocaleString() + " tokens in the last window."
          : "No cost reports at this scope.",
        cites: c,
      };
    },
  },
  {
    q: "Is the work any good?",
    keys: ["quality", "good", "review", "rework", "accept"],
    answer: (tiles) => {
      const q = tiles.filter((t) => inCategory(t, "quality"));
      const accepted = sumKpi(tiles, "quality", "accepted");
      const reviews = sumKpi(tiles, "quality", "reviews");
      const rework = sumKpi(tiles, "quality", "rework");
      return {
        text: q.length
          ? accepted + " of " + reviews + " reviews accepted, " + rework + " sent back."
          : "No quality reports at this scope.",
        cites: q,
      };
    },
  },
];

export default function ReportingAskView({ route, tiles }: Props) {
  const [asked, setAsked] = useState<string>(QUESTIONS[0].q);
  const [typed, setTyped] = useState("");

  const match = useMemo(() => {
    const exact = QUESTIONS.find((x) => x.q === asked);
    if (exact) return exact;
    return null;
  }, [asked]);

  const answer = match ? match.answer(tiles) : null;

  const submit = () => {
    const needle = typed.trim().toLowerCase();
    if (!needle) return;
    const hit = QUESTIONS.find((x) => x.keys.some((k) => needle.includes(k)));
    setAsked(hit ? hit.q : needle);
  };

  return (
    <div className="mc-ask">
      <p className="mc-proposed-note">
        <strong>Proposed.</strong> Answers are computed from the reports, not generated — the contract's stated fallback
        when no model is configured. Each one cites what it read, and a question outside the repertoire is refused
        rather than guessed at. A model should widen the repertoire, not drop the citations.
      </p>

      <div className="mc-ask-bar">
        <input
          type="search"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submit();
          }}
          placeholder="Ask about this scope…"
          aria-label="Ask a question about this scope"
        />
        <button className="mc-button mc-button-primary" type="button" onClick={submit}>
          Ask
        </button>
      </div>

      <div className="mc-ask-suggestions">
        {QUESTIONS.map((x) => (
          <button
            key={x.q}
            type="button"
            className={"mc-pill mc-ask-suggestion" + (asked === x.q ? " is-on" : "")}
            onClick={() => {
              setTyped("");
              setAsked(x.q);
            }}
          >
            {x.q}
          </button>
        ))}
      </div>

      <section className="mc-ask-answer">
        <h3>{asked}</h3>
        {answer ? (
          <>
            <p>{answer.text}</p>
            {answer.cites.length > 0 && (
              <div className="mc-ask-cites">
                <span className="mc-detail-label">Grounded in</span>
                <ul>
                  {answer.cites.map((t) => (
                    <li key={(t.scope || "") + tileName(t)}>
                      <StatusBadge status={t.status} />{" "}
                      <Link to={reportingRoute(route, { report: tileName(t), ...(t.scope ? { scope: t.scope } : {}) })}>
                        {tileName(t)}
                      </Link>
                      {t.scope && <span className="mc-body-muted mc-mono"> · {t.scope}</span>}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </>
        ) : (
          <p className="mc-empty-state">
            Not in the repertoire. Without a model this view answers a fixed set of questions rather than guessing —
            pick one above, or use words like cost, quality, delivery, decisions, or bottleneck.
          </p>
        )}
      </section>
    </div>
  );
}
