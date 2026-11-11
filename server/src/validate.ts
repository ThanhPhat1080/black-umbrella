export function validUsername(s: unknown): s is string {
  return typeof s === "string" && /^[a-z0-9_]{3,20}$/.test(s);
}

export function validPin(s: unknown): s is string {
  return typeof s === "string" && /^[0-9]{6}$/.test(s);
}

export function normalizeDisplayName(s: unknown): string | null {
  if (typeof s !== "string") return null;
  if (/[\u0000-\u001f\u007f-\u009f\u2028\u2029]/.test(s.trim())) return null;
  const t = s.trim();
  const n = [...t].length;
  return n >= 1 && n <= 32 ? t : null;
}

export function validPublicKey(b64: unknown): b64 is string {
  if (typeof b64 !== "string" || !/^[A-Za-z0-9+/]+={0,2}$/.test(b64)) return false;
  try {
    return atob(b64).length === 32;
  } catch {
    return false;
  }
}

export function dmConv(a: string, b: string): string {
  return a < b ? `dm:${a}|${b}` : `dm:${b}|${a}`;
}

const ROOM_NAME = /^[\p{L}\p{M}\p{N} !@#$%^&*()_\-+=~`]+$/u;

/** Trimmed room name, or null when it is not 1-32 characters of letters, digits, spaces and ! @ # $ % ^ & * ( ) _ - + = ~ ` */
export function validRoomName(s: unknown): string | null {
  if (typeof s !== "string") return null;
  const t = s.trim();
  const n = [...t].length;
  return n >= 1 && n <= 32 && ROOM_NAME.test(t) ? t : null;
}

export const ROOM_ID = /^room:[0-9a-f]{12}$/;

export type Conv = { kind: "hell" } | { kind: "dm"; a: string; b: string } | { kind: "room"; id: string };

export function parseConv(conv: unknown): Conv | null {
  if (conv === "hell") return { kind: "hell" };
  if (typeof conv === "string" && ROOM_ID.test(conv)) return { kind: "room", id: conv };
  if (typeof conv !== "string" || !conv.startsWith("dm:")) return null;
  const parts = conv.slice(3).split("|");
  if (parts.length !== 2) return null;
  const [a, b] = parts;
  if (!validUsername(a) || !validUsername(b) || !(a < b)) return null;
  return { kind: "dm", a, b };
}
