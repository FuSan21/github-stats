// SVG string helpers shared by every card.

const ENTITIES: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };

export const esc = (s: unknown) => String(s).replace(/[&<>"']/g, (c) => ENTITIES[c]!);

export const clamp = (n: number, min: number, max: number) => Math.min(Math.max(n, min), max);

export const truncate = (s: string, max: number) => (s.length > max ? s.slice(0, max - 1).trimEnd() + "…" : s);

export const fmt = (n: number) => n.toLocaleString("en-US");

/** 1234 -> "1.2k", like readme-stats. A precision forces a fixed number of decimals. */
export function kFormat(n: number, precision?: number): string {
  if (precision !== undefined) return (n / 1000).toFixed(precision) + "k";
  if (Math.abs(n) < 1000) return String(n);
  return parseFloat((n / 1000).toFixed(1)) + "k";
}

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`;
}

// Per-character advance widths (in em) of the readme-stats font stack, for sizing.
// prettier-ignore
const WIDTHS = [
  0.2796875, 0.2765625, 0.3546875, 0.5546875, 0.5546875, 0.8890625, 0.665625, 0.190625,
  0.3328125, 0.3328125, 0.3890625, 0.5828125, 0.2765625, 0.3328125, 0.2765625, 0.3015625,
  0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875,
  0.5546875, 0.5546875, 0.2765625, 0.2765625, 0.584375, 0.5828125, 0.584375, 0.5546875,
  1.0140625, 0.665625, 0.665625, 0.721875, 0.721875, 0.665625, 0.609375, 0.7765625,
  0.721875, 0.2765625, 0.5, 0.665625, 0.5546875, 0.8328125, 0.721875, 0.7765625,
  0.665625, 0.7765625, 0.721875, 0.665625, 0.609375, 0.721875, 0.665625, 0.94375,
  0.665625, 0.665625, 0.609375, 0.2765625, 0.3546875, 0.2765625, 0.4765625, 0.5546875,
  0.3328125, 0.5546875, 0.5546875, 0.5, 0.5546875, 0.5546875, 0.2765625, 0.5546875,
  0.5546875, 0.221875, 0.240625, 0.5, 0.221875, 0.8328125, 0.5546875, 0.5546875,
  0.5546875, 0.5546875, 0.3328125, 0.5, 0.2765625, 0.5546875, 0.5, 0.721875,
  0.5, 0.5, 0.5, 0.3546875, 0.259375, 0.353125, 0.5890625,
];

export const measureText = (s: string, fontSize = 10) =>
  [...s].reduce((w, c) => w + (WIDTHS[c.charCodeAt(0) - 32] ?? 0.5279276315789471), 0) * fontSize;

/** Wrap each item in a <g> offset by `gap` along a row or column. */
export const flex = (items: string[], gap: number, direction: "row" | "column" = "row") =>
  items
    .filter(Boolean)
    .map((item, i) => `<g transform="translate(${direction === "row" ? `${i * gap}, 0` : `0, ${i * gap}`})">${item}</g>`)
    .join("");

export function chunk<T>(arr: T[], size: number): T[][] {
  const out: T[][] = [];
  arr.forEach((item, i) => (out[Math.floor(i / size)] ??= []).push(item));
  return out;
}

const HEX = /^([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
export const isHex = (s: string | undefined): s is string => !!s && HEX.test(s);

/** "#rrggbb" for a bare hex value, else undefined. */
export const hexColor = (s: string | undefined) => (isHex(s) ? `#${s}` : undefined);

/** `angle,hex,hex[,hex...]` background gradients, as both readme-stats and streak accept. */
export function parseGradient(s: string | undefined): { angle: number; stops: string[] } | undefined {
  const parts = s?.split(",") ?? [];
  if (parts.length < 3 || !/^-?\d+$/.test(parts[0]!) || !parts.slice(1).every(isHex)) return undefined;
  return { angle: Number(parts[0]), stops: parts.slice(1).map((c) => `#${c}`) };
}

export const gradientDef = (id: string, g: { angle: number; stops: string[] }) =>
  `<linearGradient id="${id}" gradientTransform="rotate(${g.angle})" gradientUnits="userSpaceOnUse">${g.stops
    .map((c, i) => `<stop offset="${(i * 100) / (g.stops.length - 1)}%" stop-color="${c}"/>`)
    .join("")}</linearGradient>`;
