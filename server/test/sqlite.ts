import { DatabaseSync } from "node:sqlite";
import type { Sql } from "../src/sql";

export function memorySql(): Sql {
  const db = new DatabaseSync(":memory:");
  return {
    tx<T>(fn: () => T): T {
      db.exec("BEGIN");
      try {
        const r = fn();
        db.exec("COMMIT");
        return r;
      } catch (e) {
        db.exec("ROLLBACK");
        throw e;
      }
    },
    exec<T>(query: string, ...params: unknown[]): T[] {
      const stmt = db.prepare(query);
      if (stmt.columns().length > 0) return stmt.all(...(params as never[])) as T[];
      stmt.run(...(params as never[]));
      return [];
    },
  };
}
