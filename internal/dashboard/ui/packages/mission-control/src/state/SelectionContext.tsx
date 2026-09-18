import { useCallback, useMemo } from "react";
import { useRoute, useRouter } from "./RouterContext";
import { viewRoute } from "../lib/routes";
import { sameSelection, type Selection } from "../lib/selection";

export type { Selection } from "../lib/selection";
export { sameSelection, placeID, selectedID, selectionFromID } from "../lib/selection";

interface SelectionValue {
  selection: Selection;
  select: (next: Selection) => void;
  toggle: (next: NonNullable<Selection>) => void;
  clear: () => void;
}

export function useSelection(): SelectionValue {
  const route = useRoute();
  const { navigate } = useRouter();
  const selection = route.selected;

  const select = useCallback(
    (next: Selection) => navigate(viewRoute(route, { selected: next }), { replace: true }),
    [route, navigate],
  );
  const clear = useCallback(() => navigate(viewRoute(route, { selected: null }), { replace: true }), [route, navigate]);
  const toggle = useCallback(
    (next: NonNullable<Selection>) =>
      navigate(viewRoute(route, { selected: sameSelection(selection, next) ? null : next }), { replace: true }),
    [route, navigate, selection],
  );

  return useMemo(() => ({ selection, select, toggle, clear }), [selection, select, toggle, clear]);
}
