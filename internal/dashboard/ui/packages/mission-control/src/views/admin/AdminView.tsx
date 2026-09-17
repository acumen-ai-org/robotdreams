import { lazy, Suspense, useEffect } from "react";
import { ADMIN_SECTIONS, GROUP_LABELS, findSection, type AdminSection } from "./sections";
import { useConnection } from "../../state/ConnectionContext";
import { useRoute } from "../../state/RouterContext";
import { ConnectionPage } from "./ConnectionPage";
import { adminRoute } from "../../lib/routes";
import { Link } from "../../components/Link";

const ActivityPage = lazy(() => import("./ActivityPage").then((m) => ({ default: m.ActivityPage })));
const StoragePage = lazy(() => import("./StoragePage").then((m) => ({ default: m.StoragePage })));
const DocsSpine = lazy(() => import("./docs/DocsSpine"));
const DocsHierarchy = lazy(() => import("./docs/DocsHierarchy"));
const DocsReporting = lazy(() => import("./docs/DocsReporting"));
const DocsCategories = lazy(() => import("./docs/DocsCategories"));
const DocsModalities = lazy(() => import("./docs/DocsModalities"));
const DocsStances = lazy(() => import("./docs/DocsStances"));
const DocsPrimitives = lazy(() => import("./docs/DocsPrimitives"));
const DocsAggregation = lazy(() => import("./docs/DocsAggregation"));
const DocsScope = lazy(() => import("./docs/DocsScope"));
const DocsUpdates = lazy(() => import("./docs/DocsUpdates"));
const VocabularyPage = lazy(() => import("./VocabularyPage"));
const UpdatesPage = lazy(() => import("./UpdatesPage"));
const DesignColors = lazy(() => import("./design/Colors"));
const DesignTypography = lazy(() => import("./design/Typography"));
const DesignSpacing = lazy(() => import("./design/Spacing"));
const DesignIconography = lazy(() => import("./design/Iconography"));
const DesignMotion = lazy(() => import("./design/Motion"));
const DesignPrimitives = lazy(() => import("./design/Primitives"));
const DesignComposites = lazy(() => import("./design/Composites"));
const DesignVoice = lazy(() => import("./design/Voice"));
const DocsInfographic = lazy(() => import("./docs/DocsInfographic"));
const DocsLibrary = lazy(() => import("./docs/DocsLibrary"));

function Body({ id }: { id: string }) {
  switch (id) {
    case "connection":
      return <ConnectionPage />;
    case "docs/infographic":
      return <DocsInfographic />;
    case "docs/spine":
      return <DocsSpine />;
    case "docs/hierarchy":
      return <DocsHierarchy />;
    case "docs/reporting":
      return <DocsReporting />;
    case "docs/categories":
      return <DocsCategories />;
    case "docs/stances":
      return <DocsStances />;
    case "docs/modalities":
      return <DocsModalities />;
    case "docs/primitives":
      return <DocsPrimitives />;
    case "docs/aggregation":
      return <DocsAggregation />;
    case "docs/scope":
      return <DocsScope />;
    case "docs/updates":
      return <DocsUpdates />;
    case "docs/library":
      return <DocsLibrary />;
    case "vocabulary":
      return <VocabularyPage />;
    case "activity":
      return <ActivityPage />;
    case "storage":
      return <StoragePage />;
    case "updates":
      return <UpdatesPage />;
    case "design/colors":
      return <DesignColors />;
    case "design/typography":
      return <DesignTypography />;
    case "design/spacing":
      return <DesignSpacing />;
    case "design/iconography":
      return <DesignIconography />;
    case "design/motion":
      return <DesignMotion />;
    case "design/primitives":
      return <DesignPrimitives />;
    case "design/composites":
      return <DesignComposites />;
    case "design/voice":
      return <DesignVoice />;
    default:
      return <ConnectionPage />;
  }
}

function Menu({ current, sections }: { current: string; sections: AdminSection[] }) {
  const groups: AdminSection["group"][] = ["settings", "design", "docs"];
  return (
    <nav className="mc-admin-menu" aria-label="Admin sections">
      {groups.map((g) => (
        <div className="mc-admin-menu-group" key={g}>
          <span className="mc-admin-menu-heading">{GROUP_LABELS[g]}</span>
          <ul>
            {sections
              .filter((s) => s.group === g)
              .map((s) => (
                <li key={s.id}>
                  <Link
                    to={adminRoute(s.id)}
                    className={s.id === current ? "is-active" : undefined}
                    aria-current={s.id === current ? "page" : undefined}
                  >
                    {s.label}
                  </Link>
                </li>
              ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}

interface Props {
  hidden?: boolean;
  onClose: () => void;
}

export function AdminView({ hidden: hiddenProp, onClose }: Props) {
  const route = useRoute();
  const hidden = hiddenProp ?? route.view !== "admin";
  const section = route.section;
  const { canConnect } = useConnection();
  const sections = canConnect ? ADMIN_SECTIONS : ADMIN_SECTIONS.filter((s) => s.id !== "connection");
  const current = findSection(section, sections);

  useEffect(() => {
    if (hidden) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [hidden, onClose]);

  return (
    <main id="view-admin" className="mc-admin-overlay" hidden={hidden}>
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div className="mc-admin-backdrop" onClick={onClose} />
      <div className="mc-admin-dialog" role="dialog" aria-modal="true" aria-label="Settings">
        <div className="mc-admin-dialog-head">
          <span className="mc-admin-dialog-title">Settings</span>
          <button className="mc-icon-button" type="button" aria-label="Close settings" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="mc-admin-shell">
          <Menu current={current.id} sections={sections} />
          <div className="mc-admin-content">
            <header className="mc-admin-head">
              <h1>{current.label}</h1>
              <p className="mc-body-muted">{current.blurb}</p>
            </header>
            <Suspense fallback={<p className="mc-empty-state">Loading…</p>}>
              <Body id={current.id} />
            </Suspense>
          </div>
        </div>
      </div>
    </main>
  );
}
