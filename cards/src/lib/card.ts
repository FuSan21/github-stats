// The card frame and colour handling of readme-stats, used by the stats,
// top-languages and error cards.

import THEMES from "../themes/readme-stats.json";
import type { Params } from "./params.ts";
import { esc, flex, gradientDef, hexColor, parseGradient } from "./svg.ts";

type Theme = Partial<Record<"title_color" | "icon_color" | "text_color" | "bg_color" | "border_color" | "ring_color", string>>;
const themes: Record<string, Theme> = THEMES;

export type CardColors = {
  titleColor: string;
  textColor: string;
  iconColor: string;
  ringColor: string;
  borderColor: string;
  /** A colour, or a gradient for the card background. */
  bg: string | { angle: number; stops: string[] };
};

/** Theme colours with per-colour query overrides, falling back to the default theme. */
export function cardColors(p: Params): CardColors {
  const base = themes.default!;
  const theme = themes[p.str("theme") ?? "default"] ?? base;
  const pick = (key: keyof Theme, fallback: string) => hexColor(p.str(key)) ?? hexColor(theme[key]) ?? fallback;

  const titleColor = pick("title_color", `#${base.title_color}`);
  const bgParam = p.str("bg_color");
  return {
    titleColor,
    textColor: pick("text_color", `#${base.text_color}`),
    iconColor: pick("icon_color", `#${base.icon_color}`),
    ringColor: pick("ring_color", titleColor),
    borderColor: pick("border_color", `#${theme.border_color ?? base.border_color}`),
    bg: parseGradient(bgParam) ?? hexColor(bgParam) ?? hexColor(theme.bg_color) ?? `#${base.bg_color}`,
  };
}

export type CardOptions = {
  width: number;
  height: number;
  title: string;
  colors: CardColors;
  hideTitle?: boolean;
  hideBorder?: boolean;
  borderRadius?: number;
  animations?: boolean;
  css?: string;
  a11yTitle?: string;
  a11yDesc?: string;
};

/** Common frame options read from the query: title, border, radius, animations. */
export const frameOptions = (p: Params) => ({
  customTitle: p.str("custom_title"),
  hideTitle: p.bool("hide_title"),
  hideBorder: p.bool("hide_border"),
  borderRadius: p.num("border_radius"),
  animations: !p.bool("disable_animations"),
});

const PADDING_X = 25;
const PADDING_Y = 35;

export function renderCard(o: CardOptions, body: string): string {
  const height = o.hideTitle ? o.height - 30 : o.height;
  const { colors } = o;
  const fill = typeof colors.bg === "string" ? colors.bg : "url(#gradient)";
  return `<svg width="${o.width}" height="${height}" viewBox="0 0 ${o.width} ${height}" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="descId">
<title id="titleId">${esc(o.a11yTitle ?? o.title)}</title>
<desc id="descId">${esc(o.a11yDesc ?? "")}</desc>
<style>
  .header { font: 600 18px 'Segoe UI', Ubuntu, Sans-Serif; fill: ${colors.titleColor}; animation: fadeInAnimation 0.8s ease-in-out forwards; }
  @supports(-moz-appearance: auto) { .header { font-size: 15.5px; } }
  ${o.css ?? ""}
  @keyframes scaleInAnimation { from { transform: translate(-5px, 5px) scale(0); } to { transform: translate(-5px, 5px) scale(1); } }
  @keyframes fadeInAnimation { from { opacity: 0; } to { opacity: 1; } }
  ${o.animations === false ? "* { animation-duration: 0s !important; animation-delay: 0s !important; }" : ""}
</style>
${typeof colors.bg === "string" ? "" : `<defs>${gradientDef("gradient", colors.bg)}</defs>`}
<rect x="0.5" y="0.5" rx="${o.borderRadius ?? 4.5}" height="99%" width="${o.width - 1}" stroke="${colors.borderColor}" fill="${fill}" stroke-opacity="${o.hideBorder ? 0 : 1}"/>
${o.hideTitle ? "" : `<g transform="translate(${PADDING_X}, ${PADDING_Y})">${flex([`<text x="0" y="0" class="header">${esc(o.title)}</text>`], 25)}</g>`}
<g transform="translate(0, ${o.hideTitle ? PADDING_X : PADDING_Y + 20})">
${body}
</g>
</svg>`;
}

/** A readme-stats style error card. */
export function errorCard(message: string, colors: CardColors, secondary = ""): string {
  const W = 576.5;
  const fill = typeof colors.bg === "string" ? colors.bg : colors.bg.stops[0];
  return `<svg width="${W}" height="120" viewBox="0 0 ${W} 120" fill="${fill}" xmlns="http://www.w3.org/2000/svg">
<style>
  .text { font: 600 16px 'Segoe UI', Ubuntu, Sans-Serif; fill: ${colors.titleColor} }
  .small { font: 600 12px 'Segoe UI', Ubuntu, Sans-Serif; fill: ${colors.textColor} }
</style>
<rect x="0.5" y="0.5" width="${W - 1}" height="99%" rx="4.5" fill="${fill}" stroke="${colors.borderColor}"/>
<text x="25" y="45" class="text">Something went wrong!</text>
<text x="25" y="70" class="small">${esc(message)}</text>
${secondary ? `<text x="25" y="92" class="small">${esc(secondary)}</text>` : ""}
</svg>`;
}
