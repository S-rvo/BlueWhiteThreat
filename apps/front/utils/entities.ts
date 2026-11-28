export type EntityKind =
  | "Article"
  | "IP"
  | "Domain"
  | "Hash"
  | "CVE"
  | "Malware"
  | "ThreatActor"
  | "Infrastructure";

export interface GraphNode {
  id: string;
  kind: EntityKind;
  label: string;
}

export interface GraphEdge {
  from: string;
  to: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface KindMeta {
  label: string;
  plural: string;
  color: string;
  family: "ioc" | "intel" | "source";
}

// Une couleur par type d'entité, partagée par le graphe, les légendes et les tableaux.
export const KINDS: Record<EntityKind, KindMeta> = {
  IP: { label: "IP", plural: "Adresses IP", color: "#4fc3ff", family: "ioc" },
  Domain: { label: "Domaine", plural: "Domaines", color: "#4ee0a5", family: "ioc" },
  Hash: { label: "Hash", plural: "Empreintes", color: "#b39bff", family: "ioc" },
  CVE: { label: "CVE", plural: "Vulnérabilités", color: "#ff6b7d", family: "intel" },
  Malware: { label: "Malware", plural: "Malwares", color: "#ffa24c", family: "intel" },
  ThreatActor: { label: "Acteur", plural: "Acteurs de la menace", color: "#ffd84d", family: "intel" },
  Infrastructure: { label: "Infra", plural: "Infrastructures", color: "#ff7ad9", family: "intel" },
  Article: { label: "Article", plural: "Articles", color: "#8b97ad", family: "source" },
};

export const ENTITY_KINDS = (Object.keys(KINDS) as EntityKind[]).filter((k) => k !== "Article");

export interface EntityRow extends GraphNode {
  mentions: number;
}

// Entités (hors articles) avec le nombre d'articles qui les citent, triées par fréquence.
export function entityRows(graph: Graph | null | undefined): EntityRow[] {
  if (!graph) return [];
  const mentions = new Map<string, number>();
  for (const edge of graph.edges) mentions.set(edge.from, (mentions.get(edge.from) ?? 0) + 1);
  return graph.nodes
    .filter((n) => n.kind !== "Article")
    .map((n) => ({ ...n, mentions: mentions.get(n.id) ?? 0 }))
    .sort((a, b) => b.mentions - a.mentions || a.label.localeCompare(b.label));
}

export function countByKind(rows: GraphNode[]): Record<EntityKind, number> {
  const out = Object.fromEntries(Object.keys(KINDS).map((k) => [k, 0])) as Record<EntityKind, number>;
  for (const row of rows) out[row.kind] = (out[row.kind] ?? 0) + 1;
  return out;
}

// Affichage « défangé » des indicateurs réseau : un IOC copié depuis l'interface
// ne doit pas devenir un lien cliquable par accident.
export function defang(kind: EntityKind, value: string): string {
  if (kind === "IP" || kind === "Domain") return value.replace(/\./g, "[.]");
  return value;
}
