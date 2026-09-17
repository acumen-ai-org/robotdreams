import { reportingRoute, STANCES, STANCE_QUESTIONS, type Route } from "../../lib/routes";
import { Tabs, type TabDef } from "../../components/Tabs";

export function StanceControl({ route }: { route: Route }) {
  const tabs: TabDef[] = [
    { id: "", label: "any", hint: "Read the report as written", to: reportingRoute(route, { stance: "" }) },
    ...STANCES.map((s) => ({
      id: s,
      label: s,
      hint: STANCE_QUESTIONS[s],
      to: reportingRoute(route, { stance: s }),
    })),
  ];
  return <Tabs tabs={tabs} active={route.stance} label="Read this report as" variant="switch" />;
}
