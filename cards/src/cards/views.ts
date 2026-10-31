// GET /views: a profile view counter badge (replaces komarev.com/ghpvc).
// The count is kept in memory and flushed to a JSON file every few seconds.

import { mkdir, rename } from "node:fs/promises";
import { dirname } from "node:path";
import { env } from "../lib/env.ts";
import type { Params } from "../lib/params.ts";
import { esc, fmt } from "../lib/svg.ts";

let views = 0;
let dirty = false;

export async function loadViews() {
  try {
    views = ((await Bun.file(env.viewsFile).json()) as { views: number }).views;
    dirty = false;
  } catch {
    // First start: seed from INITIAL_VIEWS so the count carries over from komarev.
    views = env.initialViews;
    dirty = true;
  }
}

export async function flushViews() {
  if (!dirty) return;
  dirty = false;
  await mkdir(dirname(env.viewsFile), { recursive: true });
  const tmp = `${env.viewsFile}.tmp`;
  await Bun.write(tmp, JSON.stringify({ views }));
  await rename(tmp, env.viewsFile);
}

export function countView() {
  views += 1;
  dirty = true;
  return views;
}

const COLORS: Record<string, string> = {
  brightgreen: "#4c1", green: "#97ca00", yellow: "#dfb317", yellowgreen: "#a4a61d",
  orange: "#fe7d37", red: "#e05d44", blue: "#007ec6", grey: "#555", lightgrey: "#9f9f9f",
};

// Rough Verdana 11px advance widths, good enough to size a badge.
const textWidth = (s: string) =>
  [...s].reduce((w, c) => w + (/[il.,:;'|!]/.test(c) ? 3.2 : /[mwMW]/.test(c) ? 9.5 : /[A-Z0-9]/.test(c) ? 7.5 : 6.4), 0);

export function viewsBadge(count: number, p: Params): string {
  const label = p.str("label") ?? "Profile views";
  const raw = p.str("color") ?? "blue";
  const color = COLORS[raw] ?? (/^[0-9a-f]{3,8}$/i.test(raw) ? `#${raw}` : COLORS.blue);
  const value = fmt(count);
  const lw = Math.round(textWidth(label) + 12);
  const vw = Math.round(textWidth(value) + 12);
  const w = lw + vw;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="20" role="img" aria-label="${esc(label)}: ${value}">
<title>${esc(label)}: ${value}</title>
<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
<clipPath id="r"><rect width="${w}" height="20" rx="3" fill="#fff"/></clipPath>
<g clip-path="url(#r)"><rect width="${lw}" height="20" fill="#555"/><rect x="${lw}" width="${vw}" height="20" fill="${color}"/><rect width="${w}" height="20" fill="url(#s)"/></g>
<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
<text x="${lw / 2}" y="15" fill="#010101" fill-opacity=".3">${esc(label)}</text><text x="${lw / 2}" y="14">${esc(label)}</text>
<text x="${lw + vw / 2}" y="15" fill="#010101" fill-opacity=".3">${value}</text><text x="${lw + vw / 2}" y="14">${value}</text>
</g></svg>`;
}
