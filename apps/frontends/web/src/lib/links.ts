import type { Notes } from './api';

interface Token { text: string; path?: string | null }
type Block = [tag: string, parts: Token[]];

// Resolve only vault-local Markdown files. Wiki paths are vault-relative;
// a bare wiki title first matches a sibling, then a unique title in the vault.
export function target(from: string, raw: string, wiki: boolean, notes: Notes): string | null {
  let path = raw.trim().split("#")[0];
  if (!wiki) {
    try { path = decodeURIComponent(path); } catch { return null; }
  }
  if (!path || /[:\\?\x00-\x1f]/.test(path) || path.startsWith("/")) return null;
  if (wiki && !path.endsWith(".md")) path += ".md";
  if (!path.endsWith(".md")) return null;
  let parts = wiki && path.includes("/") && !path.startsWith(".") ? [] : from.split("/").slice(0,-1);
  for (const part of path.split("/")) {
    if (part === ".") continue;
    if (part === "..") { if (!parts.length) return null; parts.pop(); }
    else if (!part || part.startsWith(".")) return null;
    else parts.push(part);
  }
  const result = parts.join("/");
  if (wiki && !path.includes("/") && !Object.hasOwn(notes,result)) {
    const matches = Object.keys(notes).filter(n => n === path || n.endsWith("/"+path));
    if (matches.length === 1) return matches[0];
    if (matches.length > 1) return null;
  }
  return result;
}

export function tokens(line: string, from: string, notes: Notes): Token[] {
  const out: Token[] = [];
  let pos = 0;
  for (const m of line.matchAll(/`[^`]*`|\[\[([^\]|]+)(?:\|([^\]]+))?\]\]|\[([^\]]+)\]\(([^)]+)\)/g)) {
    out.push({text:line.slice(pos,m.index)});
    const dest = m[1] ?? m[4];
    const path = dest && target(from,dest,!!m[1],notes);
    out.push({text:path ? (m[2] ?? m[1] ?? m[3]) : m[0], path});
    pos = m.index+m[0].length;
  }
  out.push({text:line.slice(pos)});
  return out;
}

export function* blocks(text: string, from: string, notes: Notes): Generator<Block> {
  let code = false;
  for (const line of text.split(/\r?\n/)) {
    if (/^\s*```/.test(line)) { code = !code; continue; }
    if (code) { yield ["pre", [{text:line}]]; continue; }
    const heading = /^(#{1,6}) (.*)$/.exec(line);
    if (line) yield [heading ? "h"+heading[1].length : "p",tokens(heading ? heading[2] : line,from,notes)];
  }
}
