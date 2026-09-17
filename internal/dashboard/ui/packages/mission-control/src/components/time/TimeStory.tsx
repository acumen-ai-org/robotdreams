import { useEffect, useMemo, useState } from "react";
import { TimeAxis, type TimeAxisDirection, type TimeAxisItem } from "./TimeAxis";
import { ChevronLeftIcon, ChevronRightIcon } from "../icons";
import { fmtTime } from "../../lib/format";

export function TimeStory({
  items,
  empty,
  direction = "x",
}: {
  items: TimeAxisItem[];
  empty?: string;
  direction?: TimeAxisDirection;
}) {
  const vertical = direction === "y";
  const [picked, setPicked] = useState<string | null>(null);

  const ids = useMemo(() => items.map((i) => i.id), [items]);
  useEffect(() => {
    setPicked((cur) => (cur && ids.includes(cur) ? cur : ids.length ? ids[ids.length - 1] : null));
  }, [ids]);

  const at = picked ? items.findIndex((it) => it.id === picked) : -1;
  const prev = at > 0 ? items[at - 1].id : null;
  const next = at >= 0 && at < items.length - 1 ? items[at + 1].id : null;
  const item = at >= 0 ? items[at] : undefined;

  const segs = (item?.scope || "").split("/").filter(Boolean);
  const sev = (item?.severity || "info").toLowerCase();

  return (
    <div className={"mc-story" + (vertical ? " is-vertical" : "")}>
      <div className="mc-story-read">
        <button
          className="mc-story-step"
          type="button"
          disabled={!prev}
          onClick={() => prev && setPicked(prev)}
          aria-label="Earlier event"
          title="Earlier event"
        >
          <ChevronLeftIcon size={30} />
        </button>

        <div className="mc-story-body">
          {!item ? (
            <p className="mc-empty-state">{empty || "Nothing to show yet."}</p>
          ) : (
            <article className="mc-story-event" aria-live="polite">
              <span className="mc-story-when mc-mono">{item.t ? fmtTime(item.t) : "—"}</span>
              <h3 className="mc-story-headline">{item.headline}</h3>
              <span className="mc-story-meta">
                <span className={"mc-sev mc-sev-" + sev}>{sev}</span>
                {item.type && <span className="mc-story-type mc-mono">{item.type}</span>}
              </span>
              {segs.length > 0 && (
                <span className="mc-story-scope">
                  <span className="mc-story-site">{segs[segs.length - 1]}</span>
                  {segs.length > 1 && <span className="mc-story-parents mc-mono">{segs.slice(0, -1).join(" › ")}</span>}
                </span>
              )}
              {at >= 0 && (
                <span className="mc-story-count mc-label-muted mc-mono">
                  {at + 1} of {items.length}
                </span>
              )}
            </article>
          )}
        </div>

        <button
          className="mc-story-step"
          type="button"
          disabled={!next}
          onClick={() => next && setPicked(next)}
          aria-label="Later event"
          title="Later event"
        >
          <ChevronRightIcon size={30} />
        </button>
      </div>

      <TimeAxis items={items} direction={direction} selected={picked} onSelect={setPicked} empty={empty} />
    </div>
  );
}

export default TimeStory;
