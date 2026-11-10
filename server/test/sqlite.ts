import { DatabaseSync } from "node:sqlite";
import type { Sql } from "../src/sql";

export function memorySql(): Sql {
  const db = new DatabaseSync(":memory:");
  return {
    exec<T>(query: string, ...params: unknown[]): T[] {
      const stmt = db.prepare(query);
      if (stmt.columns().length > 0) return stmt.all(...(params as never[])) as T[];
      stmt.run(...(params as never[]));
      return [];
    },
  };
}
