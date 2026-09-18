import { useLive } from "../../state/LiveContext";
import { ActivityList } from "../overview/ActivityList";

export function ActivityPage() {
  const { activity, stream } = useLive();
  return (
    <div className="mc-admin-page">
      <p className="mc-section-abstract">
        Everything the control plane has reported since this page was opened: nodes joining, leaving and moving, the
        messages between them, and update announcements. It is a live tail held in memory — closing the tab forgets it.
        {stream !== "live" && " The event stream is not connected, so nothing new will arrive."}
      </p>
      <ActivityList entries={activity} />
    </div>
  );
}
