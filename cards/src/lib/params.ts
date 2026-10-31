// Typed access to query parameters, using the upstream apps' conventions.

export class Params {
  constructor(private readonly q: URLSearchParams) {}

  str(name: string): string | undefined {
    const v = this.q.get(name);
    return v === null || v === "" ? undefined : v;
  }

  /** Only "true" and "false" are recognised; anything else is the default. */
  bool(name: string, fallback = false): boolean {
    const v = this.q.get(name);
    return v === "true" ? true : v === "false" ? false : fallback;
  }

  num(name: string): number | undefined {
    const v = this.str(name);
    if (v === undefined) return undefined;
    const n = Number(v);
    return Number.isFinite(n) ? n : undefined;
  }

  int(name: string): number | undefined {
    const n = this.num(name);
    return n === undefined ? undefined : Math.trunc(n);
  }

  /** Comma-separated list, trimmed, empties dropped. */
  list(name: string): string[] {
    return (this.str(name) ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }
}
