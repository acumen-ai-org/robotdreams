import { useEffect, useState } from "react";
import { useRollout } from "../../hooks/useRollout";
import { RolloutPanel } from "../overview/RolloutPanel";

export default function UpdatesPage() {
  const [kind, setKind] = useState("");
  const rollout = useRollout(kind, 0, true);
  useEffect(() => {
    if (!kind && rollout.kinds.length) setKind(rollout.kinds[0]);
  }, [kind, rollout.kinds]);

  return (
    <div className="mc-admin-updates">
      <p className="mc-body-muted">
        Announcing is a push: <code>dream updates announce --kind &lt;kind&gt; --version &lt;v&gt;</code> tells every
        node a version is available, and each node decides for itself what to do about it. What comes back is below —
        and silence is an answer worth reading, so it is counted, not omitted.
      </p>
      <RolloutPanel state={rollout} kind={kind} onSelectKind={setKind} />
    </div>
  );
}
