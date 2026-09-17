import { MissionControlProviders, type MissionControlProvidersProps } from "./MissionControlProviders";
import { MissionControlShell, type MissionControlShellProps } from "./MissionControlShell";

export type MissionControlProps = Omit<MissionControlProvidersProps, "children"> & MissionControlShellProps;

export function MissionControl({ brand, brandLabel, title, redirectToConnection, ...providers }: MissionControlProps) {
  return (
    <MissionControlProviders {...providers}>
      <MissionControlShell
        brand={brand}
        brandLabel={brandLabel}
        title={title}
        redirectToConnection={redirectToConnection}
      />
    </MissionControlProviders>
  );
}
