// Minimal well-formedness check: balanced tags, quoted attributes, no stray "<" or "&".
export function assertWellFormed(svg: string) {
  const stack: string[] = [];
  const body = svg.replace(/<style>[\s\S]*?<\/style>/g, "<style/>");
  const tag = /<(\/?)([a-zA-Z][\w:-]*)((?:\s+[\w:-]+="[^"<]*")*)\s*(\/?)>/g;
  let last = 0;
  for (const m of body.matchAll(tag)) {
    const text = body.slice(last, m.index);
    if (text.includes("<")) throw new Error(`malformed markup near: ${text.slice(text.indexOf("<"), text.indexOf("<") + 60)}`);
    if (/&(?!(amp|lt|gt|quot|#39|#\d+);)/.test(text)) throw new Error(`unescaped & in: ${text.trim().slice(0, 60)}`);
    last = m.index! + m[0].length;
    const [, close, name, , selfClose] = m;
    if (selfClose) continue;
    if (close) {
      const open = stack.pop();
      if (open !== name) throw new Error(`</${name}> closes <${open}>`);
    } else stack.push(name!);
  }
  if (body.slice(last).includes("<")) throw new Error("malformed markup at end");
  if (stack.length) throw new Error(`unclosed: ${stack.join(", ")}`);
  if (!body.trimStart().startsWith("<svg")) throw new Error("not an svg document");
}
