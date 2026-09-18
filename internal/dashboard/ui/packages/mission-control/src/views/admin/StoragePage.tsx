import { useLive } from "../../state/LiveContext";
import { StorageTable } from "../overview/StorageTable";

export function StoragePage() {
  const { storageVersion } = useLive();
  return (
    <div className="mc-admin-page">
      <p className="mc-section-abstract">
        Every object in shared storage, newest first. One node deposits a book and another takes it out; every write is
        attributed, which is what lets the side panel narrow this to a single node or place.
      </p>
      <StorageTable version={storageVersion} />
    </div>
  );
}
