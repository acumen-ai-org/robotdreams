import { useEffect, useMemo, useRef, useState } from "react";
import { Canvas, useFrame, useThree, type ThreeEvent } from "@react-three/fiber";
import { hierarchy, pack } from "d3-hierarchy";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { useConnection } from "../../state/ConnectionContext";
import { placeInScope, scopeOf, type WorkerIndex } from "../../hooks/useWorkers";
import { entityCode, nodeCode } from "../../lib/vocabulary";
import type { ScopeEntry, Worker } from "../../lib/types";
import { DelayedLoading } from "../../components/Async";
import { ClearSelection } from "../../components/ZoomPan";

const PLANET_BLUE = "#4a90c4";
const PLANET_PINK = "#c07fa2";
const REALM_HEX = [
  "#1C5CAB",
  "#9A3D12",
  "#0E5F43",
  "#755000",
  "#92375D",
  "#2A6410",
  "#40329B",
  "#A02524",
  "#0E5B76",
  "#3E4A57",
];
const SITE_GREY = "#c3cbd4";
const NODE_ON = "#3f8fbf";
const NODE_OFF = "#98a4b2";

const FACTORY_BODY_TOP = 0.68;
const FACTORY_OUTLINE: [number, number][] = [
  [0, 0],
  [0, 0.68],
  [0.14, 0.92],
  [0.14, 0.68],
  [0.31, 0.92],
  [0.31, 0.68],
  [0.48, 0.92],
  [0.48, 0.68],
  [0.65, 0.92],
  [0.65, 0.68],
  [0.78, 0.68],
  [0.78, 1],
  [0.92, 1],
  [0.92, 0.68],
  [1, 0.68],
  [1, 0],
];
const FACTORY_GEO = (() => {
  const shape = new THREE.Shape();
  FACTORY_OUTLINE.forEach(([x, y], i) => (i ? shape.lineTo(x - 0.5, y) : shape.moveTo(x - 0.5, y)));
  shape.closePath();
  const geo = new THREE.ExtrudeGeometry(shape, { depth: 1, bevelEnabled: false });
  geo.translate(0, 0, -0.5);
  return geo;
})();

interface Place {
  id: string;
  name: string;
  path: string;
  level: number;
  children: Place[];
  nodes: Worker[];
}

function buildTree(workers: Worker[], scopes: ScopeEntry[]): { roots: Place[]; loose: Worker[] } {
  const roots: Place[] = [];
  const byRel = new Map<string, Place>();
  const loose: Worker[] = [];

  const placeFor = (all: string[]): Place | undefined => {
    const segs = all.slice(1);
    let rel = "";
    let full = all[0] || "";
    let list = roots;
    let node: Place | undefined;
    for (let i = 0; i < segs.length && i < 3; i++) {
      rel = rel ? rel + "/" + segs[i] : segs[i];
      full = full ? full + "/" + segs[i] : segs[i];
      let next = byRel.get(rel);
      if (!next) {
        next = { id: "scope:" + full, name: segs[i], path: full, level: i + 1, children: [], nodes: [] };
        byRel.set(rel, next);
        list.push(next);
      }
      node = next;
      list = next.children;
    }
    return node;
  };

  for (const s of scopes) placeFor(s.path.split("/").filter(Boolean));

  for (const w of workers) {
    const place = placeFor(scopeOf(w).split("/").filter(Boolean));
    if (!place) {
      loose.push(w);
      continue;
    }
    place.nodes.push(w);
  }

  const sortRec = (ps: Place[]) => {
    ps.sort((a, b) => a.name.localeCompare(b.name));
    ps.forEach((p) => sortRec(p.children));
  };
  sortRec(roots);
  return { roots, loose };
}

type Vec = [number, number, number];

interface NodeViz {
  id: string;
  label: string;
  connected: boolean;
  pos: Vec;
  r: number;
}
interface SiteViz {
  id: string;
  label: string;
  path: string;
  pos: Vec;
  w: number;
  h: number;
  d: number;
  nodes: NodeViz[];
}
interface RealmViz {
  id: string;
  label: string;
  path: string;
  pos: Vec;
  r: number;
  color: string;
  sites: SiteViz[];
  nodes: NodeViz[];
}
interface WorldViz {
  id: string;
  label: string;
  path: string;
  pos: Vec;
  r: number;
  color: string;
  realms: RealmViz[];
  nodes: NodeViz[];
}

const countWorkers = (p: Place): number => p.nodes.length + p.children.reduce((n, c) => n + countWorkers(c), 0);

function looseRow(
  workers: Worker[],
  code: (w: Worker) => string,
  cx: number,
  y: number,
  cz: number,
  span: number,
  r: number,
): NodeViz[] {
  const pitch = Math.min(r * 3, workers.length > 1 ? span / workers.length : span);
  const x0 = cx - ((workers.length - 1) * pitch) / 2;
  return workers.map((w, i) => ({
    id: w.id,
    label: `${code(w)} · ${w.id}${w.role ? " (" + w.role + ")" : ""}`,
    connected: w.status === "connected",
    pos: [x0 + i * pitch, y, cz] as Vec,
    r,
  }));
}

function factoryFloor(
  workers: Worker[],
  code: (w: Worker) => string,
  pos: Vec,
  w: number,
  h: number,
  d: number,
): NodeViz[] {
  const n = workers.length;
  if (!n) return [];
  const fw = w * 0.82;
  const fd = d * 0.82;
  const cols = Math.max(1, Math.min(n, Math.round(Math.sqrt((n * fw) / fd)) || 1));
  const rows = Math.ceil(n / cols);
  const cell = Math.min(fw / cols, fd / rows);
  const r = Math.min(cell * 0.34, h * FACTORY_BODY_TOP * 0.22);
  return workers.map((wk, i) => {
    const row = Math.floor(i / cols);
    const col = i % cols;
    const inRow = Math.min(cols, n - row * cols);
    const indent = ((cols - inRow) * cell) / 2;
    return {
      id: wk.id,
      label: `${code(wk)} · ${wk.id}${wk.role ? " (" + wk.role + ")" : ""}`,
      connected: wk.status === "connected",
      pos: [
        pos[0] - (cols * cell) / 2 + indent + (col + 0.5) * cell,
        pos[1] + r,
        pos[2] - (rows * cell) / 2 + (row + 0.5) * cell,
      ] as Vec,
      r,
    };
  });
}

function layout(roots: Place[], loose: Worker[], code: (w: Worker) => string) {
  const worldR = roots.map((w) => WORLD_BASE_R + WORLD_R_PER_SQRT_WORKER * Math.sqrt(countWorkers(w)));
  const circumference = worldR.reduce((n, r) => n + r * WORLD_RING_SPACING, 0);
  const ring = roots.length > 1 ? Math.max(circumference / (2 * Math.PI), Math.max(...worldR) * 2.2) : 0;

  let realmSeq = 0;
  let siteSeq = 0;
  const siteTotal = roots.reduce((n, w) => n + w.children.reduce((m, r) => m + r.children.length, 0), 0);

  const worlds: WorldViz[] = roots.map((world, wi) => {
    const R = worldR[wi];
    const a = (wi / Math.max(1, roots.length)) * Math.PI * 2;
    const pos: Vec = [Math.cos(a) * ring, 0, Math.sin(a) * ring];

    const disc = R * DISC;
    const slots = packRealms(world.children, disc);

    const realms: RealmViz[] = world.children.map((realm, ri) => {
      const idx = realmSeq++ % 10;
      const slot = slots[ri];
      const pr = slot.pr;
      const rpos: Vec = [pos[0] + slot.dx, pos[1], pos[2] + slot.dz];
      const offset = Math.hypot(slot.dx, slot.dz);

      const k = realm.children.length;
      const standR = k > 1 ? pr * 0.56 : 0;
      const fw = Math.min(k > 1 ? (2 * Math.PI * standR * 0.62) / k : pr * 0.66, pr * 0.5);
      const reach = standR + 0.62 * fw;
      const need = realm.nodes.length ? 1.5 : 1;
      const radial = realm.nodes.length ? Math.max(reach, pr * 0.45) : reach;
      const ceiling = Math.sqrt(Math.max(0, (DOME * R) ** 2 - (offset + radial) ** 2)) - PLATFORM_H / 2;
      const fh = Math.max(0.02, Math.min(pr * 0.5, ceiling) / need);

      const sites: SiteViz[] = realm.children.map((site, si) => {
        const sa = (si / Math.max(1, k)) * Math.PI * 2;
        const spos: Vec = [rpos[0] + Math.cos(sa) * standR, rpos[1] + PLATFORM_H / 2, rpos[2] + Math.sin(sa) * standR];
        return {
          id: site.id,
          label: `${entityCode("S", ++siteSeq, siteTotal)} · ${site.name}`,
          path: site.path,
          pos: spos,
          w: fw,
          h: fh,
          d: fw * 0.72,
          nodes: factoryFloor(site.nodes, code, spos, fw, fh, fw * 0.72),
        };
      });

      return {
        id: realm.id,
        label: `R${idx} · ${realm.name}`,
        path: realm.path,
        pos: rpos,
        r: pr,
        color: REALM_HEX[idx],
        sites,
        nodes: looseRow(
          realm.nodes,
          code,
          rpos[0],
          rpos[1] + PLATFORM_H / 2 + fh * 1.35,
          rpos[2],
          pr * 0.9,
          Math.min(fh * 0.14, pr * 0.055),
        ),
      };
    });

    return {
      id: world.id,
      label: `${entityCode("W", wi + 1, roots.length)} · ${world.name}`,
      path: world.path,
      pos,
      r: R,
      color: wi % 2 === 0 ? PLANET_BLUE : PLANET_PINK,
      realms,
      nodes: looseRow(world.nodes, code, pos[0], pos[1] + R * 0.9, pos[2], R * 0.7, R * 0.04),
    };
  });

  const top = Math.max(...worldR, 3) * 0.9;
  return { worlds, loose: looseRow(loose, code, 0, top, 0, Math.max(ring, 3) * 0.5, 0.18) };
}

const ORIGIN: Vec = [0, 0, 0];
const IGNORE = () => null;

const PLATFORM_H = 0.14;
const DISC = 0.76;
const WORLD_BASE_R = 7.5;
const WORLD_R_PER_SQRT_WORKER = 1.6;
const WORLD_RING_SPACING = 2.9;
const REALM_GAP = 0.16;
const CAMERA_MIN_DISTANCE = 0.4;
const CAMERA_MAX_DISTANCE = 160;
const WORLD_DETAIL_NEAR = 4.5;
const WORLD_INSIDE = 1.05;
const NEAR_HYSTERESIS = 1.2;
const DOME = 0.92;

interface RealmDatum {
  v: number;
  children?: RealmDatum[];
}

function packRealms(realms: Place[], disc: number): { dx: number; dz: number; pr: number }[] {
  if (!realms.length) return [];
  const root = hierarchy<RealmDatum>(
    { v: 0, children: realms.map((r) => ({ v: Math.max(1, countWorkers(r)) })) },
    (d) => d.children,
  ).sum((d) => d.v);
  const packed = pack<RealmDatum>()
    .size([disc * 2, disc * 2])
    .padding(disc * REALM_GAP)(root);
  return (packed.children || []).map((c) => ({ dx: c.x - disc, dz: c.y - disc, pr: c.r }));
}

interface Focus {
  pos: Vec;
  dist: number;
  nonce: number;
}

function Controls({ focus }: { focus: Focus | null }) {
  const { camera, gl } = useThree();
  const ref = useRef<OrbitControls | null>(null);
  const flight = useRef<{
    from: THREE.Vector3;
    to: THREE.Vector3;
    camFrom: THREE.Vector3;
    camTo: THREE.Vector3;
    t: number;
  } | null>(null);

  useEffect(() => {
    const c = new OrbitControls(camera, gl.domElement);
    c.enableDamping = true;
    c.dampingFactor = 0.08;
    c.minDistance = CAMERA_MIN_DISTANCE;
    c.maxDistance = CAMERA_MAX_DISTANCE;
    ref.current = c;
    return () => {
      ref.current = null;
      c.dispose();
    };
  }, [camera, gl]);

  useEffect(() => {
    const c = ref.current;
    if (!focus || !c) return;
    const to = new THREE.Vector3(...focus.pos);
    const dir = camera.position.clone().sub(c.target);
    if (dir.lengthSq() < 1e-6) dir.set(0, 0.5, 1);
    dir.normalize();
    flight.current = {
      from: c.target.clone(),
      to,
      camFrom: camera.position.clone(),
      camTo: to.clone().add(dir.multiplyScalar(focus.dist)),
      t: 0,
    };
  }, [focus, camera]);

  useFrame((_, dt) => {
    const c = ref.current;
    if (!c) return;
    const f = flight.current;
    if (f) {
      f.t = Math.min(1, f.t + dt / 0.7);
      const e = f.t < 0.5 ? 2 * f.t * f.t : 1 - Math.pow(-2 * f.t + 2, 2) / 2;
      c.target.lerpVectors(f.from, f.to, e);
      camera.position.lerpVectors(f.camFrom, f.camTo, e);
      if (f.t >= 1) flight.current = null;
    }
    c.update();
  });
  return null;
}

function useNear(p: Vec, threshold: number) {
  const [near, setNear] = useState(false);
  const state = useRef(false);
  const v = useMemo(() => new THREE.Vector3(...p), [p]);
  useFrame(({ camera }) => {
    const want = camera.position.distanceTo(v) < threshold * (state.current ? NEAR_HYSTERESIS : 1);
    if (want !== state.current) {
      state.current = want;
      setNear(want);
    }
  });
  return near;
}

interface PickProps {
  id: string;
  label: string;
  onSelect: (id: string) => void;
  onHover: (label: string | null) => void;
}

function pickHandlers({ id, label, onSelect, onHover }: PickProps, setHover: (b: boolean) => void) {
  return {
    onClick: (e: ThreeEvent<MouseEvent>) => {
      e.stopPropagation();
      onSelect(id);
    },
    onPointerOver: (e: ThreeEvent<PointerEvent>) => {
      e.stopPropagation();
      setHover(true);
      onHover(label);
      document.body.style.cursor = "pointer";
    },
    onPointerOut: () => {
      setHover(false);
      onHover(null);
      document.body.style.cursor = "";
    },
  };
}

function SelectRing({ r, y = 0 }: { r: number; y?: number }) {
  return (
    <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, y, 0]}>
      <ringGeometry args={[r * 1.08, r * 1.24, 64]} />
      <meshBasicMaterial color="#1f8fd0" side={THREE.DoubleSide} transparent opacity={0.9} />
    </mesh>
  );
}

function NodeDot({
  n,
  selected,
  dimmed,
  ...pick
}: { n: NodeViz; selected: boolean; dimmed: boolean } & Omit<PickProps, "id" | "label">) {
  const [hover, setHover] = useState(false);
  const h = pickHandlers({ id: n.id, label: n.label, ...pick }, setHover);
  const color = selected ? "#1f8fd0" : n.connected ? NODE_ON : NODE_OFF;
  return (
    <group>
      <mesh position={n.pos} {...h}>
        <sphereGeometry args={[n.r, 12, 12]} />
        <meshStandardMaterial
          color={color}
          emissive={color}
          emissiveIntensity={selected ? 0.8 : hover ? 0.45 : 0.15}
          transparent
          opacity={dimmed ? 0.25 : 1}
          roughness={0.5}
        />
      </mesh>
      {!dimmed && (
        <SceneLabel
          text={n.label.split(" ")[0]}
          color="#eaf3fb"
          center={n.pos}
          reach={n.r * 1.04}
          radius={n.r}
          width={n.r}
          mode="near-outside"
        />
      )}
    </group>
  );
}

function Factory({
  site,
  selected,
  dimmed,
  open,
  ...pick
}: { site: SiteViz; selected: boolean; dimmed: boolean; open: boolean } & Omit<PickProps, "id" | "label">) {
  const [hover, setHover] = useState(false);
  const h = pickHandlers({ id: site.id, label: site.label, ...pick }, setHover);
  const color = selected ? "#e8eef5" : SITE_GREY;
  return (
    <group position={site.pos}>
      <mesh geometry={FACTORY_GEO} scale={[site.w, site.h, site.d]} {...h} raycast={open ? IGNORE : undefined}>
        <meshStandardMaterial
          color={color}
          emissive={color}
          emissiveIntensity={selected ? 0.35 : hover ? 0.2 : 0.05}
          opacity={dimmed ? 0.18 : open ? 0.26 : 1}
          transparent={dimmed || open}
          depthWrite={!open}
          roughness={0.7}
          side={THREE.DoubleSide}
        />
      </mesh>
      {selected && <SelectRing r={site.w * 0.7} y={0.01} />}
      {!dimmed && (
        <SceneLabel
          text={site.label}
          color={open ? "#cfe3f4" : "#12212f"}
          center={ORIGIN}
          reach={Math.max(site.w, site.d) * 0.82}
          radius={Math.max(site.w, site.d) / 2}
          mode={open ? "far-inside" : "near-outside"}
          minPx={LABEL_MIN_PX.site}
          width={Math.max(site.w, site.d) * 0.95}
          lift={site.h * 0.62}
        />
      )}
    </group>
  );
}

function Platform({
  realm,
  selected,
  dimmed,
  ...pick
}: { realm: RealmViz; selected: boolean; dimmed: boolean } & Omit<PickProps, "id" | "label">) {
  const [hover, setHover] = useState(false);
  const h = pickHandlers({ id: realm.id, label: realm.label, ...pick }, setHover);
  return (
    <group position={realm.pos}>
      <mesh {...h}>
        <cylinderGeometry args={[realm.r, realm.r * 0.94, PLATFORM_H, 64]} />
        <meshStandardMaterial
          color={realm.color}
          emissive={realm.color}
          emissiveIntensity={selected ? 0.5 : hover ? 0.3 : 0.1}
          transparent
          opacity={dimmed ? 0.2 : 0.92}
          roughness={0.6}
        />
      </mesh>
      {selected && <SelectRing r={realm.r} y={PLATFORM_H / 2 + 0.01} />}
      {!dimmed && (
        <SceneLabel
          text={realm.label}
          color="#dbe9f7"
          center={ORIGIN}
          reach={realm.r * 1.06}
          radius={realm.r}
          width={realm.r * 0.9}
          mode="under-front"
          lift={-(PLATFORM_H / 2 + realm.r * 0.07)}
        />
      )}
    </group>
  );
}

interface Props {
  index: WorkerIndex;
  scopes: ScopeEntry[];
  active: boolean;
  selected: string | null;
  onSelect: (id: string | null) => void;
}

export default function GalaxyMap({ index, scopes, active, selected, onSelect }: Props) {
  const frameloop = active ? "always" : ("demand" as const);
  const conn = useConnection();

  useEffect(
    () => () => {
      document.body.style.cursor = "";
    },
    [],
  );

  const { workers, error } = index;

  const total = workers.length;
  const codes = useMemo(() => {
    const m = new Map<string, string>();
    workers.forEach((w, i) => m.set(w.id, nodeCode(i + 1, total)));
    return m;
  }, [workers, total]);

  const tree = useMemo(() => buildTree(workers, scopes), [workers, scopes]);
  const scene = useMemo(() => layout(tree.roots, tree.loose, (w) => codes.get(w.id) || ""), [tree, codes]);

  const [hovered, setHovered] = useState<string | null>(null);
  const [focus, setFocus] = useState<Focus | null>(null);
  const nonce = useRef(0);
  const dim = (id: string) => !index.matchAll && !index.matches.has(id);
  const dimPlace = (path: string) => !placeInScope(index, path);

  const pick = (id: string, pos: Vec, dist: number, toggle = true) => {
    onSelect(toggle && selected === id ? null : id);
    setFocus({ pos, dist, nonce: ++nonce.current });
  };

  const clear = () => {
    onSelect(null);
    setFocus({ pos: [0, 0, 0], dist: spanOf(scene.worlds) * 2.4, nonce: ++nonce.current });
  };

  if (!conn.hasToken) return <SectionMessage>Connect to a server to see the galaxy.</SectionMessage>;
  if (error) return <SectionMessage>{error}</SectionMessage>;
  if (index.pending)
    return (
      <SectionMessage>
        <DelayedLoading />
      </SectionMessage>
    );
  if (!tree.roots.length && !tree.loose.length) return <SectionMessage>No workers connected yet.</SectionMessage>;

  const span = spanOf(scene.worlds);

  return (
    <section className="mc-panel mc-panel-fill mc-galaxy" aria-label="Galaxy map">
      <div className="mc-zoom-viewport">
        <Canvas
          camera={{ position: [0, span * 0.8, span * 2.1], fov: 46, near: 0.05, far: 600 }}
          dpr={[1, 2]}
          frameloop={frameloop}
          gl={{ alpha: true }}
        >
          <ambientLight intensity={0.85} />
          <directionalLight position={[span, span * 1.5, span]} intensity={1.6} />
          <directionalLight position={[-span, span * 0.4, -span]} intensity={0.5} />
          <Controls focus={focus} />

          {scene.loose.map((n) => (
            <NodeDot
              key={n.id}
              n={n}
              selected={selected === n.id}
              dimmed={dim(n.id)}
              onSelect={() => pick(n.id, n.pos, n.r * 14)}
              onHover={setHovered}
            />
          ))}

          {scene.worlds.map((world) => (
            <World
              key={world.id}
              world={world}
              selected={selected}
              dim={dim}
              dimPlace={dimPlace}
              onPick={pick}
              onHover={setHovered}
            />
          ))}
        </Canvas>
        <ClearSelection show={!!selected} onClear={clear} />
        <p className="mc-galaxy-hint mc-body-muted">
          {hovered || "Drag to orbit · scroll to zoom · click a world to open it, then pick what is inside"}
        </p>
      </div>
    </section>
  );
}

function World({
  world,
  selected,
  dim,
  dimPlace,
  onPick,
  onHover,
}: {
  world: WorldViz;
  selected: string | null;
  dim: (id: string) => boolean;
  dimPlace: (path: string) => boolean;
  onPick: (id: string, pos: Vec, dist: number, toggle?: boolean) => void;
  onHover: (label: string | null) => void;
}) {
  const [hover, setHover] = useState(false);
  const h = pickHandlers(
    { id: world.id, label: world.label, onSelect: () => onPick(world.id, world.pos, world.r * 3.2, false), onHover },
    setHover,
  );
  const near = useNear(world.pos, world.r * WORLD_DETAIL_NEAR);
  const inside = useNear(world.pos, world.r * WORLD_INSIDE);
  const isSel = selected === world.id;
  const open = isSel || inside;
  const dimmed = dimPlace(world.path);

  return (
    <group>
      <mesh position={world.pos} renderOrder={2} {...h} raycast={open ? IGNORE : undefined}>
        <sphereGeometry args={[world.r, 48, 32]} />
        <meshStandardMaterial
          color={world.color}
          emissive={world.color}
          emissiveIntensity={isSel ? 0.5 : hover ? 0.3 : 0.12}
          transparent
          opacity={dimmed ? 0.06 : open ? 0.1 : 0.24}
          depthWrite={false}
          roughness={0.25}
          side={THREE.DoubleSide}
        />
      </mesh>
      {isSel && (
        <group position={world.pos}>
          <SelectRing r={world.r} />
        </group>
      )}

      {!dimmed && (
        <SceneLabel
          text={world.label}
          color="#e8f2fb"
          center={world.pos}
          reach={world.r * 0.96}
          radius={world.r}
          width={world.r * 1.45}
          mode="far-inside"
          lift={world.r * 0.42}
        />
      )}

      {world.realms.map((realm) => (
        <group key={realm.id}>
          <Platform
            realm={realm}
            selected={selected === realm.id}
            dimmed={dimPlace(realm.path)}
            onSelect={() => onPick(realm.id, realm.pos, realm.r * 3.4)}
            onHover={onHover}
          />
          {realm.sites.map((site) => (
            <Site
              key={site.id}
              site={site}
              selected={selected}
              dim={dim}
              dimmed={dimPlace(site.path)}
              onPick={onPick}
              onHover={onHover}
            />
          ))}
          {near &&
            realm.nodes.map((n) => (
              <NodeDot
                key={n.id}
                n={n}
                selected={selected === n.id}
                dimmed={dim(n.id)}
                onSelect={() => onPick(n.id, n.pos, n.r * 14)}
                onHover={onHover}
              />
            ))}
        </group>
      ))}

      {world.nodes.map((n) => (
        <NodeDot
          key={n.id}
          n={n}
          selected={selected === n.id}
          dimmed={dim(n.id)}
          onSelect={() => onPick(n.id, n.pos, n.r * 14)}
          onHover={onHover}
        />
      ))}
    </group>
  );
}

type LabelMode = "far-inside" | "near-outside" | "under-front";

const LABEL_MIN_PX: Record<LabelMode | "site", number> = {
  "far-inside": 44,
  "near-outside": 13,
  "under-front": 26,
  site: 20,
};

const LABEL_FONT = '700 64px "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, monospace';

function useLabelTexture(text: string, color: string) {
  const made = useMemo(() => {
    const pad = 10;
    const probe = document.createElement("canvas").getContext("2d");
    if (!probe) return null;
    probe.font = LABEL_FONT;
    const w = Math.ceil(probe.measureText(text).width) + pad * 2;
    const h = 64 + pad * 2;
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.font = LABEL_FONT;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.shadowColor = "rgba(4,8,14,0.9)";
    ctx.shadowBlur = 10;
    ctx.fillStyle = color;
    ctx.fillText(text, w / 2, h / 2);
    ctx.fillText(text, w / 2, h / 2);
    const tex = new THREE.CanvasTexture(canvas);
    tex.anisotropy = 4;
    return { tex, aspect: w / h };
  }, [text, color]);

  useEffect(() => () => made?.tex.dispose(), [made]);
  return made;
}

interface SceneLabelProps {
  text: string;
  color: string;
  center: Vec;
  reach: number;
  radius: number;
  width: number;
  mode: LabelMode;
  minPx?: number;
  lift?: number;
}

function SceneLabel({ text, color, center, reach, radius, width, mode, minPx, lift = 0 }: SceneLabelProps) {
  const made = useLabelTexture(text, color);
  const ref = useRef<THREE.Mesh>(null);
  const mid = useRef(new THREE.Vector3());
  const dir = useRef(new THREE.Vector3());
  const base = useRef(new THREE.Vector3());

  useFrame(({ camera, size: view }) => {
    const m = ref.current;
    if (!m) return;
    if (m.parent) m.parent.getWorldPosition(base.current);
    else base.current.set(0, 0, 0);
    mid.current.set(base.current.x + center[0], base.current.y + center[1], base.current.z + center[2]);
    const dist = camera.position.distanceTo(mid.current);
    const fov = (camera as THREE.PerspectiveCamera).fov || 46;
    const perUnit = view.height / 2 / Math.tan((fov * Math.PI) / 360);
    const apparent = dist > 0 ? (radius / dist) * perUnit : 0;
    if (apparent < (minPx ?? LABEL_MIN_PX[mode])) {
      m.visible = false;
      return;
    }
    m.visible = true;

    dir.current.subVectors(camera.position, mid.current);
    if (mode === "under-front") {
      dir.current.y = 0;
    }
    dir.current.normalize();

    const along = mode === "far-inside" ? -reach : reach;
    m.position.set(
      mid.current.x + dir.current.x * along - base.current.x,
      mid.current.y + dir.current.y * along + lift - base.current.y,
      mid.current.z + dir.current.z * along - base.current.z,
    );
    m.quaternion.copy(camera.quaternion);
  });

  if (!made) return null;
  return (
    <mesh ref={ref} renderOrder={4} raycast={IGNORE}>
      <planeGeometry args={[width, width / made.aspect]} />
      <meshBasicMaterial map={made.tex} transparent depthWrite={false} toneMapped={false} />
    </mesh>
  );
}

function Site({
  site,
  selected,
  dim,
  dimmed,
  onPick,
  onHover,
}: {
  site: SiteViz;
  selected: string | null;
  dim: (id: string) => boolean;
  dimmed: boolean;
  onPick: (id: string, pos: Vec, dist: number, toggle?: boolean) => void;
  onHover: (label: string | null) => void;
}) {
  const isSel = selected === site.id;
  const open = isSel || (selected != null && site.nodes.some((n) => n.id === selected));
  return (
    <group>
      <Factory
        site={site}
        selected={isSel}
        dimmed={dimmed}
        open={open}
        onSelect={() => onPick(site.id, [site.pos[0], site.pos[1] + site.h * 0.35, site.pos[2]], site.w * 4)}
        onHover={onHover}
      />
      {open &&
        site.nodes.map((n) => (
          <NodeDot
            key={n.id}
            n={n}
            selected={selected === n.id}
            dimmed={dim(n.id)}
            onSelect={() => onPick(n.id, n.pos, Math.max(n.r * 14, 1))}
            onHover={onHover}
          />
        ))}
    </group>
  );
}

function spanOf(worlds: WorldViz[]) {
  return Math.max(...worlds.map((w) => Math.hypot(w.pos[0], w.pos[2]) + w.r), 6);
}

function SectionMessage({ children }: { children: React.ReactNode }) {
  return (
    <section className="mc-panel mc-panel-fill" aria-label="Galaxy map">
      <p className="mc-empty-state">{children}</p>
    </section>
  );
}
